// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	"github.com/pkg/errors"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/hostcapabilities"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/agentcomponents"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/componentchecker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/httpserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
)

type (
	// apiKeyValidator is a function that validates the API key, provided to newExtension for mocking
	apiKeyValidator func(context.Context, string, *zap.Logger, *datadog.APIClient) error
	// sourceProviderGetter is a function that returns a source.Provider, provided to newExtension for mocking
	sourceProviderGetter func(component.TelemetrySettings, string, time.Duration) (source.Provider, error)
	// forwarderGetter is a function that returns a defaultforwarder.Forwarder, provided to newExtension for mocking
	forwarderGetter func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder
)

// defaultForwarderInterface is wrapper for methods in datadog-agent DefaultForwarder struct
type defaultForwarderInterface interface {
	defaultforwarder.Forwarder
	Start() error
	State() uint32
	Stop()
}

// eventSourcePair pairs a component status event with its source component ID
type eventSourcePair struct {
	source *componentstatus.InstanceID
	event  *componentstatus.Event
}

type fleetAutomationExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	extensionConfig          *Config
	extensionID              component.ID
	telemetry                component.TelemetrySettings
	collectorConfig          *confmap.Conf
	collectorConfigStringMap map[string]any
	ticker                   *time.Ticker
	ctxWithCancel            context.Context
	cancel                   context.CancelFunc
	uuid                     uuid.UUID

	buildInfo  payload.CustomBuildInfo
	moduleInfo service.ModuleInfos
	version    string

	forwarder  defaultForwarderInterface
	compressor *compression.Compressor
	serializer serializer.MetricSerializer

	agentMetadataPayload payload.AgentMetadata
	otelMetadataPayload  payload.OtelMetadata
	otelCollectorPayload payload.OtelCollector

	httpServer      *httpserver.Server
	componentStatus map[string]any // retrieved from StatusWatcher interface

	hostnameProvider source.Provider
	hostnameSource   string // can be "config", or "inferred"

	// Fields for implementing PipelineWatcher interface
	eventCh chan *eventSourcePair
	readyCh chan struct{}
	host    component.Host

	componentStatusMux sync.Mutex
}

var (
	_ extensioncapabilities.ConfigWatcher   = (*fleetAutomationExtension)(nil)
	_ extensioncapabilities.PipelineWatcher = (*fleetAutomationExtension)(nil)
)

// NotifyConfig implements the ConfigWatcher interface, which allows this extension
// to be notified of the Collector's effective configuration. See interface:
// https://github.com/open-telemetry/opentelemetry-collector/blob/d0fde2f6b98f13cbbd8657f8188207ac7d230ed5/extension/extension.go#L46.
//
// This method is called during the startup process by the Collector's Service right after
// calling Start.
func (e *fleetAutomationExtension) NotifyConfig(ctx context.Context, conf *confmap.Conf) error {

	e.collectorConfig = conf
	e.telemetry.Logger.Info("Received new collector configuration")
	e.collectorConfigStringMap = e.collectorConfig.ToStringMap()
	hostname, err := getHostname(ctx, e.hostnameSource, e.hostnameProvider, e.extensionConfig)
	if err != nil {
		return err
	}
	// create agent metadata payload. most fields are not relevant to OSS collector.
	e.agentMetadataPayload = payload.PrepareAgentMetadataPayload(
		e.extensionConfig.API.Site,
		e.buildInfo.Command,
		e.buildInfo.Version,
		e.buildInfo.Version,
		hostname,
	)

	// convert full config map to a json string and remove excess quotation marks
	fullConfig := componentchecker.DataToFlattenedJSONString(e.collectorConfigStringMap, false, false)

	// create otel metadata payload
	e.otelMetadataPayload = payload.PrepareOtelMetadataPayload(
		e.buildInfo.Version,
		e.version,
		e.buildInfo.Command,
		fullConfig,
	)

	e.otelCollectorPayload = payload.PrepareOtelCollectorPayload(
		hostname,
		e.hostnameSource,
		e.uuid.String(),
		e.version,
		e.extensionConfig.API.Site,
		fullConfig,
		e.buildInfo,
	)
	// send payloads to Datadog backend
	_, err = httpserver.PrepareAndSendFleetAutomationPayloads(
		e.telemetry.Logger,
		e.serializer,
		e.forwarder,
		hostname,
		e.uuid.String(),
		e.componentStatus,
		e.moduleInfo,
		e.collectorConfigStringMap,
		e.agentMetadataPayload,
		e.otelMetadataPayload,
		e.otelCollectorPayload,
	)
	if err != nil {
		return err
	}

	return nil
}

// Start starts the extension via the component interface.
func (e *fleetAutomationExtension) Start(ctx context.Context, host component.Host) error {
	err := e.forwarder.Start()
	if err != nil {
		return err
	}

	// Store the host for component status tracking
	e.host = host

	if m, ok := host.(hostcapabilities.ModuleInfo); ok {
		e.moduleInfo = m.GetModuleInfos()
	} else {
		e.telemetry.Logger.Warn("Collector component/module info not available; Datadog Fleet Automation will only show the active collector config")
	}

	hostname, err := getHostname(e.ctxWithCancel, e.hostnameSource, e.hostnameProvider, e.extensionConfig)
	if err != nil {
		return err
	}

	// Create and start HTTP server
	if e.extensionConfig.HTTPConfig.Enabled {
		e.httpServer = httpserver.NewServer(e.telemetry.Logger, e.serializer, e.forwarder, e.extensionConfig.HTTPConfig)
		if e.httpServer == nil {
			return fmt.Errorf("failed to create HTTP server")
		}
		e.httpServer.Start(func(w http.ResponseWriter, r *http.Request) {
			httpserver.HandleMetadata(
				w,
				e.telemetry.Logger,
				e.hostnameSource,
				hostname,
				e.uuid.String(),
				&e.componentStatus,
				&e.moduleInfo,
				&e.collectorConfigStringMap,
				&e.agentMetadataPayload,
				&e.otelMetadataPayload,
				&e.otelCollectorPayload,
				e.serializer,
				e.forwarder,
			)
		})
	}
	// Create a ticker that triggers every 20 minutes (FA has 1 hour TTL)
	// Start a goroutine that will send the Datadog fleet automation payload every 20 minutes
	go e.sendPayloadsOnTicker(hostname)
	return nil
}

func (e *fleetAutomationExtension) sendPayloadsOnTicker(hostname string) {
	if e.ticker == nil {
		return
	}
	for {
		select {
		case <-e.ticker.C:
			// Send fleet automation payload(s) periodically
			_, err := httpserver.PrepareAndSendFleetAutomationPayloads(
				e.telemetry.Logger,
				e.serializer,
				e.forwarder,
				hostname,
				e.uuid.String(),
				e.componentStatus,
				e.moduleInfo,
				e.collectorConfigStringMap,
				e.agentMetadataPayload,
				e.otelMetadataPayload,
				e.otelCollectorPayload,
			)
			if err != nil {
				e.telemetry.Logger.Error("Failed to prepare and send fleet automation payloads", zap.Error(err))
			}
		case <-e.ctxWithCancel.Done():
			e.telemetry.Logger.Info("Stopping datadog fleet automation payload sender")
			e.ticker.Stop()
			return
		}
	}
}

// processComponentStatusEvents processes component status events and updates the componentStatus map
func (e *fleetAutomationExtension) processComponentStatusEvents() {
	// Record events with component.StatusStarting, but queue other events until
	// PipelineWatcher.Ready is called. This prevents aggregate statuses from
	// flapping between StatusStarting and StatusOK as components are started
	// individually by the service.
	var eventQueue []*eventSourcePair

	for loop := true; loop; {
		select {
		case esp, ok := <-e.eventCh:
			if !ok {
				return
			}
			if esp.event.Status() != componentstatus.StatusStarting {
				eventQueue = append(eventQueue, esp)
				continue
			}
			e.updateComponentStatus(esp)
		case <-e.readyCh:
			for _, esp := range eventQueue {
				e.updateComponentStatus(esp)
			}
			eventQueue = nil
			loop = false
		case <-e.ctxWithCancel.Done():
			return
		}
	}

	// After PipelineWatcher.Ready, record statuses as they are received.
	for {
		select {
		case esp, ok := <-e.eventCh:
			if !ok {
				return
			}
			e.updateComponentStatus(esp)
		case <-e.ctxWithCancel.Done():
			return
		}
	}
}

// updateComponentStatus updates the componentStatus map with the latest status
func (e *fleetAutomationExtension) updateComponentStatus(esp *eventSourcePair) {
	e.componentStatusMux.Lock()
	defer e.componentStatusMux.Unlock()

	if e.componentStatus == nil {
		e.componentStatus = make(map[string]any)
	}

	componentKey := fmt.Sprintf("%s:%s", strings.ToLower(esp.source.Kind().String()), esp.source.ComponentID())
	e.componentStatus[componentKey] = map[string]any{
		"status":    esp.event.Status().String(),
		"error":     esp.event.Err(),
		"timestamp": esp.event.Timestamp(),
	}
}

// Shutdown stops the extension via the component interface.
// It shuts down the HTTP server, stops forwarder, and passes signal on
// channel to end goroutine that sends the Datadog fleet automation payloads.
func (e *fleetAutomationExtension) Shutdown(ctx context.Context) error {
	// Preemptively send the stopped event, so it can be exported before shutdown
	componentstatus.ReportStatus(e.host, componentstatus.NewEvent(componentstatus.StatusStopped))

	close(e.eventCh)
	if e.httpServer != nil {
		e.httpServer.Stop()
	}
	e.cancel()
	e.forwarder.Stop()
	e.telemetry.Logger.Info("Stopped Datadog Fleet Automation extension")
	return nil
}

func getHostname(ctx context.Context, hostnameSource string, sp source.Provider, cfg *Config) (string, error) {
	if hostnameSource == "config" {
		return cfg.Hostname, nil
	}
	source, err := sp.Source(ctx)
	if err != nil {
		return "", errors.Wrap(err, "hostname detection failed, please set hostname manually in config")
	}
	return source.Identifier, nil
}

func newExtension(
	ctx context.Context,
	config *Config,
	settings extension.Settings,
	apiKeyValidator apiKeyValidator,
	sourceProviderGetter sourceProviderGetter,
	forwarderGetter forwarderGetter,
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
	var hostnameSource string
	if config.Hostname == "" {
		hostnameSource = "inferred"
	} else {
		hostnameSource = "config"
	}
	sp, err := sourceProviderGetter(telemetry, config.Hostname, 15*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "hostname detection failed to start, hostname must be set manually in config")
	}
	hn, err := sp.Source(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "hostname detection failed to start, hostname must be set manually in config")
	}
	cfg := agentcomponents.NewConfigComponent(telemetry, string(config.API.Key), config.API.Site)
	log := agentcomponents.NewLogComponent(telemetry)

	// Initialize forwarder, compressor, and serializer components to forward OTel Inventory to REDAPL backend
	forwarder, ok := forwarderGetter(cfg, log).(defaultForwarderInterface)
	if !ok {
		return nil, fmt.Errorf("failed to create forwarder")
	}
	compressor := agentcomponents.NewCompressor()
	serializer := agentcomponents.NewSerializer(forwarder, compressor, cfg, log, hn.Identifier)
	version := settings.BuildInfo.Version
	buildInfo := payload.CustomBuildInfo{
		Command:     settings.BuildInfo.Command,
		Description: settings.BuildInfo.Description,
		Version:     settings.BuildInfo.Version,
	}
	ctxWithCancel, cancel := context.WithCancel(ctx)
	e := &fleetAutomationExtension{
		extensionID:      settings.ID,
		extensionConfig:  config,
		telemetry:        telemetry,
		collectorConfig:  &confmap.Conf{},
		forwarder:        forwarder,
		compressor:       &compressor,
		serializer:       serializer,
		buildInfo:        buildInfo,
		version:          version,
		ticker:           time.NewTicker(defaultReporterPeriod),
		hostnameProvider: sp,
		hostnameSource:   hostnameSource,
		uuid:             extUUID,
		// Initialize PipelineWatcher fields
		eventCh:       make(chan *eventSourcePair),
		readyCh:       make(chan struct{}),
		ctxWithCancel: ctxWithCancel,
		cancel:        cancel,
	}
	// Start processing component status events in the background
	go e.processComponentStatusEvents()
	return e, nil
}

// Ready implements the extension.PipelineWatcher interface.
func (e *fleetAutomationExtension) Ready() error {
	close(e.readyCh)
	return nil
}

// NotReady implements the extension.PipelineWatcher interface.
func (e *fleetAutomationExtension) NotReady() error {
	return nil
}

// ComponentStatusChanged implements the extension.StatusWatcher interface.
func (e *fleetAutomationExtension) ComponentStatusChanged(
	source *componentstatus.InstanceID,
	event *componentstatus.Event,
) {
	// There can be late arriving events after shutdown. We need to close
	// the event channel so that this function doesn't block and we release all
	// goroutines, but attempting to write to a closed channel will panic; log
	// and recover.
	defer func() {
		if r := recover(); r != nil {
			e.telemetry.Logger.Info(
				"discarding event received after shutdown",
				zap.Any("source", source),
				zap.Any("event", event),
			)
		}
	}()
	e.eventCh <- &eventSourcePair{source: source, event: event}
}
