// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
)

type fleetAutomationExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	extensionConfig          *Config
	telemetry                component.TelemetrySettings
	collectorConfig          *confmap.Conf
	collectorConfigStringMap map[string]any
	ticker                   *time.Ticker
	done                     chan bool
	mu                       sync.RWMutex

	buildInfo      component.BuildInfo
	moduleInfo     service.ModuleInfos
	moduleInfoJSON moduleInfoJSON
	version        string
	id             component.ID

	forwarder  *defaultforwarder.DefaultForwarder
	compressor *compression.Compressor
	serializer *serializer.Serializer

	agentMetadataPayload AgentMetadata
	otelMetadataPayload  OtelMetadata
	hostMetadataPayload  HostMetadata

	httpServer           *http.Server
	healthCheckV2Enabled bool
	healthCheckV2Config  map[string]any
	componentStatus      map[string]any // retrieved from healthcheckv2 extension, if enabled/configured
}

var _ extensioncapabilities.ConfigWatcher = (*fleetAutomationExtension)(nil)

// NotifyConfig implements the ConfigWatcher interface, which allows this extension
// to be notified of the Collector's effective configuration. See interface:
// https://github.com/open-telemetry/opentelemetry-collector/blob/d0fde2f6b98f13cbbd8657f8188207ac7d230ed5/extension/extension.go#L46.

// This method is called during the startup process by the Collector's Service right after
// calling Start.
func (e *fleetAutomationExtension) NotifyConfig(_ context.Context, conf *confmap.Conf) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.collectorConfig = conf
	e.telemetry.Logger.Info("Received new collector configuration")
	e.collectorConfigStringMap = e.collectorConfig.ToStringMap()

	// check if healthcheckV2 is configured, enabled, and properly configured
	// if so, set healthCheckV2Enabled to true
	healthCheckV2Configured := e.isComponentConfigured("healthcheckv2", extensionsType)
	if healthCheckV2Configured {
		e.healthCheckV2Config = e.getComponentConfig("healthcheckv2", extensionsType)
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
		if e.isModuleAvailable("healthcheckv2", extensionType) {
			e.telemetry.Logger.Info("healthcheckv2 extension is included with your collector but not configured; component status will not be available in Datadog Fleet page")
		}
	}

	// create a sample hostmetadata payload
	// TODO: replace with actual host info
	e.hostMetadataPayload = HostMetadata{
		CPUArchitecture:              "unknown",
		CPUCacheSize:                 9437184,
		CPUCores:                     6,
		CPUFamily:                    "6",
		CPUFrequency:                 2208.007,
		CPULogicalProcessors:         6,
		CPUModel:                     "Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz",
		CPUModelID:                   "158",
		CPUStepping:                  "10",
		CPUVendor:                    "GenuineIntel",
		KernelName:                   "Linux",
		KernelRelease:                "5.16.0-6-amd64",
		KernelVersion:                "#1 SMP PREEMPT Debian 5.16.18-1 (2022-03-29)",
		OS:                           "GNU/Linux",
		OSVersion:                    "debian bookworm/sid",
		MemorySwapTotalKB:            10237948,
		MemoryTotalKB:                12227556,
		IPAddress:                    "192.168.24.138",
		IPv6Address:                  "fe80::1ff:fe23:4567:890a",
		MACAddress:                   "01:23:45:67:89:AB",
		AgentVersion:                 e.buildInfo.Version,
		CloudProvider:                "AWS",
		CloudProviderSource:          "DMI",
		CloudProviderAccountID:       "aws_account_id",
		CloudProviderHostID:          "32809141302",
		HypervisorGuestUUID:          "ec24ce06-9ac4-42df-9c10-14772aeb06d7",
		DMIProductUUID:               "ec24ce06-9ac4-42df-9c10-14772aeb06d7",
		DMIAssetTag:                  "i-abcedf",
		DMIAssetVendor:               "Amazon EC2",
		LinuxPackageSigningEnabled:   true,
		RPMGlobalRepoGPGCheckEnabled: false,
	}

	// create agent metadata payload. most fields are not relevant to OSS collector.
	e.agentMetadataPayload = AgentMetadata{
		AgentVersion:                           "7.64.0-collector",
		AgentStartupTimeMs:                     1738781602921,
		AgentFlavor:                            "agent",
		ConfigAPMDDUrl:                         "",
		ConfigSite:                             e.extensionConfig.API.Site,
		ConfigLogsDDUrl:                        "",
		ConfigLogsSocks5ProxyAddress:           "",
		ConfigNoProxy:                          make([]string, 0),
		ConfigProcessDDUrl:                     "",
		ConfigProxyHTTP:                        "",
		ConfigProxyHTTPS:                       "",
		ConfigEKSFargate:                       false,
		InstallMethodTool:                      e.buildInfo.Command,
		InstallMethodToolVersion:               e.buildInfo.Version,
		InstallMethodInstallerVersion:          e.buildInfo.Version,
		LogsTransport:                          "",
		FeatureFIPSEnabled:                     false,
		FeatureCWSEnabled:                      false,
		FeatureCWSNetworkEnabled:               false,
		FeatureCWSSecurityProfilesEnabled:      false,
		FeatureCWSRemoteConfigEnabled:          false,
		FeatureCSMVMContainersEnabled:          false,
		FeatureCSMVMHostsEnabled:               false,
		FeatureContainerImagesEnabled:          false,
		FeatureProcessEnabled:                  false,
		FeatureProcessesContainerEnabled:       false,
		FeatureProcessLanguageDetectionEnabled: false,
		FeatureNetworksEnabled:                 false,
		FeatureNetworksHTTPEnabled:             false,
		FeatureNetworksHTTPSEnabled:            false,
		FeatureLogsEnabled:                     false,
		FeatureCSPMEnabled:                     false,
		FeatureAPMEnabled:                      false,
		FeatureRemoteConfigurationEnabled:      true,
		FeatureOTLPEnabled:                     true,
		FeatureIMDSv2Enabled:                   false,
		FeatureUSMEnabled:                      false,
		FeatureUSMKafkaEnabled:                 false,
		FeatureUSMJavaTLSEnabled:               false,
		FeatureUSMGoTLSEnabled:                 false,
		FeatureUSMHTTPByStatusCodeEnabled:      false,
		FeatureUSMHTTP2Enabled:                 false,
		FeatureUSMIstioEnabled:                 false,
		ECSFargateTaskARN:                      "",
		ECSFargateClusterName:                  "",
		Hostname:                               metadata.Type.String(),
		FleetPoliciesApplied:                   make([]string, 0),
	}

	// convert full config map to a json string and remove excess quotation marks
	configJSON, err := json.MarshalIndent(e.collectorConfigStringMap, "", "  ")
	if err != nil {
		e.telemetry.Logger.Error("Failed to marshal collector config", zap.Error(err))
		return nil
	}
	fullConfig := string(configJSON)
	fullConfig = strings.ReplaceAll(fullConfig, "\"", "")

	e.otelMetadataPayload = OtelMetadata{
		Enabled:                          true,
		Version:                          e.buildInfo.Version,
		ExtensionVersion:                 e.version,
		Command:                          e.buildInfo.Command,
		Description:                      "OSS Collector with Datadog Fleet Automation Extension",
		ProvidedConfiguration:            "", // This gets overwritten by populateModuleInfoJSON in http.go
		RuntimeOverrideConfiguration:     "",
		EnvironmentVariableConfiguration: "", // This gets overwritten by getHealchCheckStatus in http.go
		FullConfiguration:                fullConfig,
	}

	// handleMetadata sends the payload(s) to the Datadog backend
	go e.handleMetadata(nil, nil)

	return nil
}

// Start starts the extension via the component interface.
func (e *fleetAutomationExtension) Start(_ context.Context, host component.Host) error {
	if e.forwarder != nil {
		err := e.forwarder.Start()
		if err != nil {
			e.telemetry.Logger.Error("Failed to start forwarder", zap.Error(err))
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

func newExtension(config *Config, settings extension.Settings) *fleetAutomationExtension {
	telemetry := settings.TelemetrySettings

	cfg := newConfigComponent(telemetry, config)
	log := newLogComponent(telemetry)
	// Initialize forwarder, compressor, and serializer components to forward OTel Inventory to REDAPL backend
	forwarder := newForwarder(cfg, log)
	compressor := newCompressor()
	serializer := newSerializer(forwarder, compressor, cfg)
	version := settings.BuildInfo.Version
	return &fleetAutomationExtension{
		extensionConfig: config,
		telemetry:       telemetry,
		collectorConfig: &confmap.Conf{},
		forwarder:       forwarder,
		compressor:      &compressor,
		serializer:      serializer,
		buildInfo:       settings.BuildInfo,
		id:              settings.ID,
		version:         version,
		ticker:          time.NewTicker(20 * time.Minute),
		done:            make(chan bool),
	}
}
