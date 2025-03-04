// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
)

type (
	// APIKeyValidator is a function that validates the API key, provided to newExtension for mocking
	APIKeyValidator func(context.Context, string, *zap.Logger, *datadog.APIClient) error
	// SourceProviderGetter is a function that returns a source.Provider, provided to newExtension for mocking
	SourceProviderGetter func(component.TelemetrySettings, string, time.Duration) (source.Provider, error)
	// ForwarderGetter is a function that returns a defaultforwarder.Forwarder, provided to newExtension for mocking
	ForwarderGetter func(coreconfig.Component, corelog.Component, string) defaultforwarder.Forwarder
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
	uuid                     uuid.UUID

	buildInfo            component.BuildInfo
	moduleInfo           service.ModuleInfos
	moduleInfoJSON       *moduleInfoJSON
	activeComponentsJSON *activeComponentsJSON
	version              string

	forwarder  defaultForwarderInterface
	compressor *compression.Compressor
	serializer serializer.MetricSerializer

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
	componentChecker ComponentChecker
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

	e.updateHostname(ctx)

	e.checkHealthCheckV2()

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
	// TODO: update to use the GetModuleInfos() from `service/hostcapabilities` module
	type exportModules interface {
		GetModuleInfos() service.ModuleInfos
	}

	if host, ok := host.(exportModules); ok {
		e.moduleInfo = host.GetModuleInfos()
	} else {
		e.telemetry.Logger.Warn("Collector component/module info not available; Datadog Fleet Automation will only show the active collector config")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata", e.handleMetadata)
	e.httpServer = &http.Server{
		Addr:         ":" + fmt.Sprintf("%d", serverPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,  // Set read timeout to 5 seconds
		WriteTimeout: 10 * time.Second, // Set write timeout to 10 seconds
	}
	e.startLocalConfigServer()

	e.telemetry.Logger.Info("Started Datadog Fleet Automation extension")
	return nil
}

// Shutdown stops the extension via the component interface.
// It shuts down the HTTP server, stops forwarder, and passes signal on
// channel to end goroutine that sends the Datadog fleet automation payloads.
func (e *fleetAutomationExtension) Shutdown(ctx context.Context) error {
	e.stopLocalConfigServer()
	e.forwarder.Stop()
	e.telemetry.Logger.Info("Stopped Datadog Fleet Automation extension")
	return nil
}

func getHostname(ctx context.Context, providedHostname string, sp source.Provider) (hostname string, hostnameSource string, sourceProviderError error) {
	hostnameSource = "config"
	hostname = providedHostname
	if hostname == "" {
		source, err := sp.Source(ctx)
		if err != nil {
			err = fmt.Errorf("hostname detection failed, please set hostname manually in config: %w", err)
			return "", "unset", err
		}
		hostname = source.Identifier
		hostnameSource = "inferred"
	}
	return hostname, hostnameSource, nil
}

func (e *fleetAutomationExtension) updateHostname(ctx context.Context) {
	// check for new hostname in extension config
	// TODO: switch to conf.Sub() method on refactor
	eCfg, err := e.collectorConfig.Sub(e.extensionID.String())
	if err != nil || len(eCfg.AllKeys()) == 0 {
		e.telemetry.Logger.Error("Failed to get extension config", zap.Error(err))
	} else {
		hostname := eCfg.Get("hostname")
		if hostname, ok := hostname.(string); ok {
			if hostname != "" {
				if hostname != e.hostname {
					e.hostname = hostname
					e.hostnameSource = "config"
				}
			} else {
				e.telemetry.Logger.Info("Hostname in config is empty, inferring hostname")
				hn, source, err := getHostname(ctx, e.hostname, e.hostnameProvider)
				if err != nil {
					e.telemetry.Logger.Error("Failed to infer hostname, collector will not show in Fleet Automation", zap.Error(err))
					e.hostname = ""
					e.hostnameSource = "unset"
				} else {
					e.hostname = hn
					e.telemetry.Logger.Info("Inferred hostname", zap.String("hostname", e.hostname))
					e.hostnameSource = source
				}
			}
		}
	}
}

func (e *fleetAutomationExtension) checkHealthCheckV2() {
	// check if healthcheckV2 is configured, enabled, and properly configured
	// if so, set healthCheckV2Enabled to true
	healthCheckV2Configured, healthCheckV2ID := e.componentChecker.isComponentConfigured("healthcheckv2", extensionsKind)
	if healthCheckV2ID != nil {
		e.healthCheckV2ID = healthCheckV2ID
	}
	if healthCheckV2Configured {
		extensionsConf, err := e.collectorConfig.Sub(extensionsKind)
		if err != nil || len(extensionsConf.AllKeys()) == 0 {
			e.telemetry.Logger.Error("Failed to get extensions config", zap.Error(err))
		}
		healthCheckV2Conf, err := extensionsConf.Sub(e.healthCheckV2ID.String())
		if err != nil || len(healthCheckV2Conf.AllKeys()) == 0 {
			// This should never happen because we already got the exact component ID above with isComponentConfigured
			e.telemetry.Logger.Error("Failed to get healthcheckv2 config", zap.Error(err))
			return
		}
		e.healthCheckV2Config = healthCheckV2Conf.ToStringMap()

		// if healthCheckV2 is in config, are the settings configured properly to enable health check functionality?
		e.healthCheckV2Enabled, err = e.componentChecker.isHealthCheckV2Enabled()
		if err != nil {
			e.telemetry.Logger.Warn(err.Error())
		} else if !e.healthCheckV2Enabled {
			e.telemetry.Logger.Info("healthcheckv2 extension is included in your collector config but not properly configured")
		}
	} else if e.componentChecker.isModuleAvailable("healthcheckv2", extensionKind) {
		e.telemetry.Logger.Info("healthcheckv2 extension is included with your collector but not configured; component status will not be available in Datadog Fleet page")
	}
}

func newExtension(
	ctx context.Context,
	config *Config,
	settings extension.Settings,
	apiKeyValidator APIKeyValidator,
	sourceProviderGetter SourceProviderGetter,
	forwarderGetter ForwarderGetter,
) (*fleetAutomationExtension, error) {
	// API Key validation
	// TODO: consider moving common logic to pkg/datadog or internal/datadog
	apiClient := clientutil.CreateAPIClient(
		settings.BuildInfo,
		fmt.Sprintf("https://api.%s", config.API.Site), // TODO: does this need to be safer/more adaptable?
		config.ClientConfig)
	// Not passed as goroutine here; no sense skipping API key when all the extension does is send metadata
	// API Key is always required for proper functionality
	err := apiKeyValidator(ctx, string(config.API.Key), settings.Logger, apiClient)
	if err != nil {
		if config.API.FailOnInvalidKey {
			return nil, err
		}
		settings.Logger.Warn(err.Error())
	}
	extUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	telemetry := settings.TelemetrySettings
	// Get Hostname provider
	sp, err := sourceProviderGetter(telemetry, config.Hostname, 15*time.Second)
	if err != nil {
		telemetry.Logger.Warn("hostname detection failed to start, hostname must be set manually in config: %v", zap.Error(err))
	}
	hostname, hostnameSource, err := getHostname(ctx, config.Hostname, sp)
	if err != nil {
		telemetry.Logger.Warn(err.Error())
		return nil, err
	}

	cfg := newConfigComponent(telemetry, config)
	log := newLogComponent(telemetry)

	// Initialize forwarder, compressor, and serializer components to forward OTel Inventory to REDAPL backend
	forwarderEndpoint := "https://api." + config.API.Site
	forwarder, ok := forwarderGetter(cfg, log, forwarderEndpoint).(defaultForwarderInterface)
	if !ok {
		return nil, fmt.Errorf("failed to create forwarder")
	}
	compressor := newCompressor()
	serializer := newSerializer(forwarder, compressor, cfg, log, hostname)
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
		ticker:           time.NewTicker(config.ReporterPeriod),
		done:             make(chan bool),
		hostnameProvider: sp,
		hostnameSource:   hostnameSource,
		hostname:         hostname,
		uuid:             extUUID,
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
