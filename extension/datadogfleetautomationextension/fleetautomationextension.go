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
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
)

type fleetAutomationExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	extensionConfig *Config
	telemetry       component.TelemetrySettings
	collectorConfig *confmap.Conf
	ticker          *time.Ticker
	done            chan bool
	mu              sync.RWMutex

	buildInfo  component.BuildInfo
	moduleInfo service.ModuleInfos
	version    string
	id         component.ID

	forwarder  *defaultforwarder.DefaultForwarder
	compressor *compression.Compressor
	serializer *serializer.Serializer

	agentMetadataPayload AgentMetadata
	otelMetadataPayload  OtelMetadata
	hostMetadataPayload  HostMetadata

	httpServer *http.Server
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
		AgentVersion:                 "7.69.0",
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

	// mp := metadataPayload{
	// 	Hostname:  metadata.Type.String(),
	// 	Timestamp: time.Now().UnixNano(),
	// 	Metadata:  e.hostMetadataPayload,
	// 	UUID:      uuid.GetUUID(),
	// }

	// err = e.serializer.SendMetadata(&mp)
	// if err != nil {
	// 	e.telemetry.Logger.Error("Failed to send host metadata to Datadog backend", zap.Error(err))
	// }

	e.agentMetadataPayload = AgentMetadata{
		AgentVersion:                           "7.69.0",
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

	// moduleJSON, err := json.MarshalIndent(e.moduleInfo, "", "  ")
	// var providedModules string
	// if err != nil {
	// 	e.telemetry.Logger.Error("Failed to marshal module info", zap.Error(err))
	// 	providedModules = ""
	// } else {
	// 	providedModules = string(moduleJSON)
	// 	providedModules = strings.ReplaceAll(providedModules, "\"", "")
	// }

	configMap := e.collectorConfig.ToStringMap()
	configJSON, err := json.MarshalIndent(configMap, "", "  ")
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
		ProvidedConfiguration:            "", // TODO: Re-implement module list
		RuntimeOverrideConfiguration:     "",
		EnvironmentVariableConfiguration: "",
		FullConfiguration:                fullConfig,
	}
	// jsonBytes, err := json.MarshalIndent(inventoryOtelPayload, "", "  ")
	// if err != nil {
	// 	e.telemetry.Logger.Error("Failed to marshal JSON structure", zap.Error(err))
	// 	return nil
	// }

	// e.telemetry.Logger.Info("JSON Structure: ", zap.String("json", string(jsonBytes)))

	// p := payload{
	// 	Hostname:  metadata.Type.String(),
	// 	Timestamp: time.Now().UnixNano(),
	// 	Metadata:  e.otelMetadataPayload,
	// 	UUID:      uuid.GetUUID(),
	// }
	// e.telemetry.Logger.Info("Sending fleet automation payload to Datadog backend with:", zap.Any("metadata", p))
	// err = e.serializer.SendMetadata(&p)
	// if err != nil {
	// 	e.telemetry.Logger.Error("Failed to send fleet automation payload to Datadog backend", zap.Error(err))
	// }

	go e.handleMetadata(nil, nil)

	return nil
}

// handleMetadata writes the metadata payloads to the response writer.
// It also sends these payloads to the Datadog backend
func (e *fleetAutomationExtension) handleMetadata(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	mp := metadataPayload{
		Hostname:  metadata.Type.String(),
		Timestamp: time.Now().UnixNano(),
		Metadata:  e.hostMetadataPayload,
		UUID:      uuid.GetUUID(),
	}
	ap := agentPayload{
		Hostname:  metadata.Type.String(),
		Timestamp: time.Now().UnixNano(),
		Metadata:  e.agentMetadataPayload,
		UUID:      uuid.GetUUID(),
	}
	p := payload{
		Hostname:  metadata.Type.String(),
		Timestamp: time.Now().UnixNano(),
		Metadata:  e.otelMetadataPayload,
		UUID:      uuid.GetUUID(),
	}

	// e.serializer.SendMetadata(&mp)
	e.serializer.SendMetadata(&ap)
	e.serializer.SendMetadata(&p)

	combinedPayload := CombinedPayload{
		MetadataPayload: mp,
		AgentPayload:    ap,
		OtelPayload:     p,
	}

	// Marshal the combined payload to JSON
	jsonData, err := json.MarshalIndent(combinedPayload, "", "  ")
	if err != nil {
		http.Error(w, "Failed to marshal combined payload", http.StatusInternalServerError)
		return
	}

	if w != nil {
		// Write the JSON response
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	}

}

// Start starts the extension via the component interface.
func (e *fleetAutomationExtension) Start(_ context.Context, host component.Host) error {
	if e.forwarder != nil {
		err := e.forwarder.Start()
		if err != nil {
			e.telemetry.Logger.Error("Failed to start forwarder", zap.Error(err))
		}
	}

	// TODO: implement hostcapabilities.GetModuleInfos to get the list of modules

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata", e.handleMetadata)
	//TODO: let user specify port in config
	e.httpServer = &http.Server{
		Addr:    ":8088",
		Handler: mux,
	}

	go func() {
		if err := e.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.telemetry.Logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Create a ticker that triggers every 20 minutes (FA has 1 hour TTL)
	// Start a goroutine that will send the Datadog fleet automation payload every 20 minutes
	go func(ticker *time.Ticker) {
		for {
			select {
			case <-ticker.C:
				// Call handleMetadata periodically
				e.handleMetadata(nil, nil)
			case <-e.done:
				e.telemetry.Logger.Info("Stopping datadog fleet automation payload sender")
				ticker.Stop()
				return
			}
		}
	}(e.ticker)

	e.telemetry.Logger.Info("HTTP Server started on port 8088")

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
