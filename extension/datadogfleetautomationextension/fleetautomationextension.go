// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/hostmetadata"
)

// defaultForwarderInterface is wrapper for methods in datadog-agent DefaultForwarder struct
type defaultForwarderInterface interface {
	defaultforwarder.Forwarder
	Start() error
	State() uint32
	Stop()
}

type fleetAutomationExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	extensionConfig          *Config
	extensionID              component.ID
	telemetry                component.TelemetrySettings
	collectorConfig          *confmap.Conf
	collectorConfigStringMap map[string]any
	ticker                   *time.Ticker
	done                     chan bool
	mu                       sync.RWMutex

	buildInfo            component.BuildInfo
	moduleInfo           service.ModuleInfos
	moduleInfoJSON       *moduleInfoJSON
	activeComponentsJSON *activeComponentsJSON
	version              string

	forwarder  defaultForwarderInterface
	compressor *compression.Compressor
	serializer *serializer.Serializer

	agentMetadataPayload AgentMetadata
	otelMetadataPayload  OtelMetadata

	httpServer           *http.Server
	healthCheckV2Enabled bool
	healthCheckV2ID      *component.ID // currently first healthcheckv2 extension found; could expand to multiple health checks if needed
	healthCheckV2Config  map[string]any
	componentStatus      map[string]any // retrieved from healthcheckv2 extension, if enabled/configured

	hostnameProvider source.Provider
	hostnameSource   string // can be "unset", "config", or "inferred"
	hostname         string // unique identifier for host where collector is running
}

var _ extensioncapabilities.ConfigWatcher = (*fleetAutomationExtension)(nil)

// NotifyConfig implements the ConfigWatcher interface, which allows this extension
// to be notified of the Collector's effective configuration. See interface:
// https://github.com/open-telemetry/opentelemetry-collector/blob/d0fde2f6b98f13cbbd8657f8188207ac7d230ed5/extension/extension.go#L46.
//
// This method is called during the startup process by the Collector's Service right after
// calling Start.
func (e *fleetAutomationExtension) NotifyConfig(ctx context.Context, conf *confmap.Conf) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.collectorConfig = conf
	e.telemetry.Logger.Info("Received new collector configuration")
	e.collectorConfigStringMap = e.collectorConfig.ToStringMap()

	// check for new hostname in extension config
	// TODO: switch to conf.Sub() method on refactor
	extensionConfig := e.getComponentSubConfigMap(e.extensionID.String(), extensionsKind)
	if extensionConfig != nil {
		hostname := extensionConfig["hostname"]
		if hostname != e.hostname {
			if hostname != "" {
				e.hostname = hostname.(string)
				e.hostnameSource = "config"
			} else {
				e.telemetry.Logger.Info("Hostname in config is empty, inferring hostname")
				source, err := e.hostnameProvider.Source(ctx)
				if err != nil {
					e.telemetry.Logger.Error("Failed to infer hostname, collector will not show in Fleet Automation", zap.Error(err))
					e.hostname = ""
					e.hostnameSource = "unset"
				} else {
					e.hostname = source.Identifier
					e.telemetry.Logger.Info("Inferred hostname", zap.String("hostname", e.hostname))
					e.hostnameSource = "inferred"
				}
			}
		}
	}
	// check if healthcheckV2 is configured, enabled, and properly configured
	// if so, set healthCheckV2Enabled to true
	healthCheckV2Configured, healthCheckV2ID := e.isComponentConfigured("healthcheckv2", extensionsKind)
	if healthCheckV2ID != nil {
		e.healthCheckV2ID = healthCheckV2ID
	}
	if healthCheckV2Configured {
		// TODO: switch to conf.Sub() method on refactor
		e.healthCheckV2Config = e.getComponentSubConfigMap(e.healthCheckV2ID.String(), extensionsKind)
		enabled, err := e.isHealthCheckV2Enabled()
		e.healthCheckV2Enabled = false
		if err != nil {
			e.telemetry.Logger.Warn(err.Error())
		} else if !enabled {
			e.telemetry.Logger.Info("healthcheckv2 extension is included in your collector config but not properly configured")
		} else {
			e.healthCheckV2Enabled = true
		}
	} else {
		if e.isModuleAvailable("healthcheckv2", extensionKind) {
			e.telemetry.Logger.Info("healthcheckv2 extension is included with your collector but not configured; component status will not be available in Datadog Fleet page")
		}
	}

	// create agent metadata payload. most fields are not relevant to OSS collector.
	e.agentMetadataPayload = prepareAgentMetadataPayload(
		e.extensionConfig.API.Site,
		e.buildInfo.Command,
		e.buildInfo.Version,
		e.buildInfo.Version,
		e.hostname,
	)

	// convert full config map to a json string and remove excess quotation marks
	fullConfig := dataToFlattenedJSONString(e.collectorConfigStringMap, false, false)

	// create otel metadata payload
	e.otelMetadataPayload = prepareOtelMetadataPayload(
		e.buildInfo.Version,
		e.version,
		e.buildInfo.Command,
		fullConfig,
	)

	// send payloads to Datadog backend
	_, err := e.prepareAndSendFleetAutomationPayloads()
	if err != nil {
		e.telemetry.Logger.Error("Failed to prepare and send fleet automation payloads", zap.Error(err))
		return err
	}

	return nil
}

// Start starts the extension via the component interface.
func (e *fleetAutomationExtension) Start(_ context.Context, host component.Host) error {
	if e.forwarder != nil {
		err := e.forwarder.Start()
		if err != nil {
			e.telemetry.Logger.Error("Failed to start forwarder", zap.Error(err))
			return err
		}
	}

	// exportModules exposes the GetModulesInfos() private method from collector/service/internal/graph
	type exportModules interface {
		GetModuleInfos() service.ModuleInfos
	}

	if host, ok := host.(exportModules); ok {
		e.moduleInfo = host.GetModuleInfos()
	} else {
		e.telemetry.Logger.Warn("Collector component/module info not available; Datadog Fleet Automation will only show the active collector config")
	}

	err := e.startLocalConfigServer()
	if err != nil {
		e.telemetry.Logger.Warn("Failed to start local config server; local fleet metadata requests will not be available", zap.Error(err))
	}

	e.telemetry.Logger.Info("Started Datadog Fleet Automation extension")
	return nil
}

// Shutdown stops the extension via the component interface.
// It shuts down the HTTP server, stops forwarder, and passes signal on
// channel to end goroutine that sends the Datadog fleet automation payloads.
func (e *fleetAutomationExtension) Shutdown(ctx context.Context) error {
	if e.httpServer != nil {
		e.httpServer.Shutdown(ctx)
	}
	e.done <- true
	e.forwarder.Stop()
	e.telemetry.Logger.Info("Stopped Datadog Fleet Automation extension")
	return nil
}

func getHostname(ctx context.Context, telemetry component.TelemetrySettings, providedHostname string) (hostname string, hostnameSource string, sourceProvider *source.Provider, sourceProviderError error) {
	hostnameSource = "config"
	hostname = providedHostname
	sp, err := hostmetadata.GetSourceProvider(telemetry, providedHostname, 15*time.Second)
	if err != nil {
		err = fmt.Errorf("hostname detection failed to start, hostname must be set manually in config: %v", err)
		return "", "unset", nil, err
	}
	if hostname == "" {
		source, err := sp.Source(ctx)
		if err != nil {
			err = fmt.Errorf("hostname detection failed, please set hostname manually in config: %v", err)
			hostnameSource = "unset"
			return "", "unset", &sp, err
		} else {
			hostname = source.Identifier
			hostnameSource = "inferred"
		}
	}
	return hostname, hostnameSource, &sp, err
}

// func validateAPIKey(ctx context.Context, clientConfig confighttp.ClientConfig, buildInfo component.BuildInfo, logger *zap.Logger, apiKey, site string)

func newExtension(ctx context.Context, config *Config, settings extension.Settings) (*fleetAutomationExtension, error) {
	// API Key validation
	// TODO: consider moving common logic to pkg/datadog or internal/datadog
	errchan := make(chan error)
	apiClient := clientutil.CreateAPIClient(
		settings.BuildInfo,
		fmt.Sprintf("https://api.%s", config.API.Site),
		config.ClientConfig)
	go func() { errchan <- clientutil.ValidateAPIKey(ctx, string(config.API.Key), settings.Logger, apiClient) }()
	if config.API.FailOnInvalidKey {
		if err := <-errchan; err != nil {
			return nil, err
		}
	}

	telemetry := settings.TelemetrySettings
	// Get Hostname provider
	hostname, hostnameSource, sourceProvider, err := getHostname(ctx, telemetry, config.Hostname)
	if err != nil {
		telemetry.Logger.Warn(err.Error())
		return nil, err
	}

	cfg := newConfigComponent(telemetry, config)
	log := newLogComponent(telemetry)
	// Initialize forwarder, compressor, and serializer components to forward OTel Inventory to REDAPL backend
	forwarder, ok := newForwarder(cfg, log).(defaultForwarderInterface)
	if !ok {
		return nil, fmt.Errorf("failed to create forwarder")
	}
	compressor := newCompressor()
	serializer := newSerializer(forwarder, compressor, cfg)
	version := settings.BuildInfo.Version
	return &fleetAutomationExtension{
		extensionID:      settings.ID,
		extensionConfig:  config,
		telemetry:        telemetry,
		collectorConfig:  &confmap.Conf{},
		forwarder:        forwarder,
		compressor:       &compressor,
		serializer:       serializer,
		buildInfo:        settings.BuildInfo,
		version:          version,
		ticker:           time.NewTicker(20 * time.Minute),
		done:             make(chan bool),
		hostnameProvider: *sourceProvider,
		hostnameSource:   hostnameSource,
		hostname:         hostname,
	}, nil
}

func prepareAgentMetadataPayload(site, tool, toolversion, installerversion, hostname string) AgentMetadata {
	return AgentMetadata{
		AgentVersion:                      "7.64.0-collector",
		AgentStartupTimeMs:                1234567890123,
		AgentFlavor:                       "agent",
		ConfigSite:                        site,
		ConfigEKSFargate:                  false,
		InstallMethodTool:                 tool,
		InstallMethodToolVersion:          toolversion,
		InstallMethodInstallerVersion:     installerversion,
		FeatureRemoteConfigurationEnabled: true,
		FeatureOTLPEnabled:                true,
		Hostname:                          hostname,
	}
}

func prepareOtelMetadataPayload(version, extensionVersion, command, fullConfig string) OtelMetadata {
	return OtelMetadata{
		Enabled:                          true,
		Version:                          version,
		ExtensionVersion:                 extensionVersion,
		Command:                          command,
		Description:                      "OSS Collector with Datadog Fleet Automation Extension",
		ProvidedConfiguration:            "",
		EnvironmentVariableConfiguration: "",
		FullConfiguration:                fullConfig,
	}
}
