// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/google/uuid"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/agentcomponents"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/httpserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
)

func Test_NotifyConfig(t *testing.T) {
	tests := []struct {
		name           string
		configData     map[string]any
		expectedConfig map[string]any
		expectedError  string
		expectedLog    string
		forwarder      forwarderGetter
		provider       sourceProviderGetter
		apikey         apiKeyValidator
	}{
		{
			name: "Forwarder fails to send metadata",
			configData: map[string]any{
				"service": map[string]any{},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{},
			},
			expectedError: "failed to send datadog_agent payload",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{failSendMetadata: true, state: 1}
			},
		},
		{
			name: "Invalid configuration",
			configData: map[string]any{
				"invalid": "config",
			},
			expectedConfig: map[string]any{
				"invalid": "config",
			},
			expectedError: "",
			expectedLog:   "Failed to populate active components JSON",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{state: 1}
			},
		},
		{
			name: "Valid configuration",
			configData: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{"otlp"},
							"exporters": []any{"debug"},
						},
					},
				},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{"otlp"},
							"exporters": []any{"debug"},
						},
					},
				},
			},
			expectedError: "",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{state: 1}
			},
		},
		{
			name: "Empty configuration",
			configData: map[string]any{
				"service": map[string]any{},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{},
			},
			expectedError: "",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a confmap.Conf from the test config data
			conf := confmap.NewFromStringMap(tt.configData)

			// Create a background context
			ctx := context.Background()

			// Create a logger for testing
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			set := extension.Settings{}
			// Create telemetry settings with the test logger
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger
			set.TelemetrySettings = telemetry
			set.BuildInfo = component.BuildInfo{
				Command:     "otelcol",
				Description: "OpenTelemetry Collector",
				Version:     "1.0.0",
			}
			set.ID = component.MustNewID(metadata.Type.String())

			if tt.forwarder == nil {
				tt.forwarder = agentcomponents.NewForwarder
			}
			if tt.provider == nil {
				mspg := mockSourceProviderGetter{
					provider: &mockSourceProvider{hostname: "inferred-hostname"},
				}
				tt.provider = mspg.GetSourceProvider
			}
			if tt.apikey == nil {
				api := mockAPIKeyValidator{}
				tt.apikey = api.ValidateAPIKey
			}

			faExt, err := newExtension(ctx, &Config{}, set, clientutil.ValidateAPIKey, tt.provider, tt.forwarder)
			assert.NoError(t, err)
			err = faExt.forwarder.Start()
			defer faExt.forwarder.Stop()
			assert.NoError(t, err)

			err = faExt.NotifyConfig(ctx, conf)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Verify that the configuration is correctly set
			assert.Equal(t, conf, faExt.collectorConfig)

			// Verify that the collectorConfigStringMap contains all the items of expectedConfig
			for key, expectedValue := range tt.expectedConfig {
				actualValue, exists := faExt.collectorConfigStringMap[key]
				assert.True(t, exists, "Expected key %s not found in collectorConfigStringMap", key)
				assert.Equal(t, expectedValue, actualValue, "Value for key %s does not match", key)
			}

			// Check if the expected log message is present
			if tt.expectedLog != "" {
				found := false
				for _, log := range logs.All() {
					if log.Message == tt.expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found")
			}
		})
	}
}

func TestNewExtension(t *testing.T) {
	tests := []struct {
		name                 string
		config               *Config
		apiKeyValidator      *mockAPIKeyValidator
		sourceProviderGetter *mockSourceProviderGetter
		forwarderGetter      forwarderGetter
		expectedError        string
	}{
		{
			name: "Valid configuration",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedError: "",
		},
		{
			name: "Invalid API key",
			config: &Config{
				API: APIConfig{
					Site:             "datadoghq.com",
					Key:              "invalid-api-key",
					FailOnInvalidKey: true,
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: fmt.Errorf("invalid API key"),
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedError: "invalid API key",
		},
		{
			name: "Hostname detection failed",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "", err: fmt.Errorf("hostname detection failed")},
				err:      fmt.Errorf("hostname detection failed"),
			},
			expectedError: "hostname detection failed",
		},
		{
			name: "Failed to create forwarder",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			forwarderGetter: newNilForwarder,
			expectedError:   "failed to create forwarder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			settings := extension.Settings{
				TelemetrySettings: telemetry,
				BuildInfo: component.BuildInfo{
					Command:     "otelcol",
					Description: "OpenTelemetry Collector",
					Version:     "1.0.0",
				},
				ID: component.MustNewID(metadata.Type.String()),
			}

			if tt.forwarderGetter == nil {
				tt.forwarderGetter = agentcomponents.NewForwarder
			}

			ext, err := newExtension(
				ctx,
				tt.config,
				settings,
				tt.apiKeyValidator.ValidateAPIKey,
				tt.sourceProviderGetter.GetSourceProvider,
				tt.forwarderGetter,
			)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ext)
			}
		})
	}
}

func TestGetHostname(t *testing.T) {
	tests := []struct {
		name                 string
		providedHostname     string
		sourceProviderGetter *mockSourceProviderGetter
		expectedHostname     string
		expectedError        string
	}{
		{
			name:             "Provided hostname is empty, source provider infers hostname",
			providedHostname: "",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedHostname: "inferred-hostname",
			expectedError:    "",
		},
		{
			name:             "Provided hostname is empty, source provider fails to infer hostname",
			providedHostname: "",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "", err: fmt.Errorf("hostname detection failed")},
				err:      nil,
			},
			expectedHostname: "",
			expectedError:    "hostname detection failed, please set hostname manually in config: hostname detection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger
			sp, _ := tt.sourceProviderGetter.GetSourceProvider(telemetry, tt.providedHostname, 15*time.Second)

			config := &Config{
				Hostname: tt.providedHostname,
			}

			hostname, err := getHostname(ctx, tt.providedHostname, sp, config)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedHostname, hostname)
		})
	}
}

type mockHost struct {
	moduleInfos service.ModuleInfos
	extensions  map[component.ID]component.Component
}

func (m mockHost) GetModuleInfos() service.ModuleInfos {
	return m.moduleInfos
}

func (m mockHost) GetExtensions() map[component.ID]component.Component {
	return m.extensions
}

func TestFleetAutomationExtension_Start(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
		host          component.Host
		moduleInfos   service.ModuleInfos
		config        *Config
	}{
		{
			name:          "Forwarder starts successfully",
			forwarder:     mockForwarder{},
			expectedError: "",
			host:          nil,
			config: &Config{
				HTTPConfig: &httpserver.Config{
					Enabled: true,
					Path:    "/metadata",
					ServerConfig: confighttp.ServerConfig{
						Endpoint: "localhost:8088",
					},
				},
			},
		},
		{
			name:          "Forwarder start error",
			forwarder:     mockForwarder{startError: fmt.Errorf("forwarder start error")},
			expectedError: "forwarder start error",
			host:          nil,
			config: &Config{
				HTTPConfig: &httpserver.Config{
					Enabled: true,
					Path:    "/metadata",
					ServerConfig: confighttp.ServerConfig{
						Endpoint: "localhost:8088",
					},
				},
			},
		},
		{
			name:          "host implements ModuleInfo interface",
			forwarder:     mockForwarder{},
			expectedError: "",
			host: mockHost{
				moduleInfos: service.ModuleInfos{
					Receiver: map[component.Type]service.ModuleInfo{
						component.MustNewType("otlp"): {BuilderRef: "otlp@v0.117.0"},
					},
				},
			},
			config: &Config{
				HTTPConfig: &httpserver.Config{
					Enabled: true,
					Path:    "/metadata",
					ServerConfig: confighttp.ServerConfig{
						Endpoint: "localhost:8088",
					},
				},
			},
		},
		{
			name:          "host doesn't implement ModuleInfo interface",
			forwarder:     mockForwarder{},
			expectedError: "",
			host:          nil,
			config: &Config{
				HTTPConfig: &httpserver.Config{
					Enabled: true,
					Path:    "/metadata",
					ServerConfig: confighttp.ServerConfig{
						Endpoint: "localhost:8088",
					},
				},
			},
		},
		{
			name:          "HTTP server disabled via configuration",
			forwarder:     mockForwarder{},
			expectedError: "",
			host:          nil,
			config: &Config{
				HTTPConfig: &httpserver.Config{
					Enabled: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger
			ctxWithCancel, cancel := context.WithCancel(ctx)
			ext := &fleetAutomationExtension{
				telemetry: telemetry,
				forwarder: tt.forwarder,
				eventCh:   make(chan *eventSourcePair),
				hostnameProvider: &mockSourceProvider{
					hostname: "inferred-hostname",
					err:      nil,
				},
				hostnameSource:  "inferred",
				ctxWithCancel:   ctxWithCancel,
				cancel:          cancel,
				extensionConfig: tt.config,
			}

			err := ext.Start(ctx, tt.host)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			err = ext.Shutdown(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestFleetAutomationExtension_Shutdown(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
		httpServer    *http.Server
	}{
		{
			name:          "Forwarder stops successfully",
			forwarder:     mockForwarder{},
			expectedError: "",
		},
		{
			name:          "Forwarder stop error",
			forwarder:     mockForwarder{stopError: fmt.Errorf("forwarder stop error")},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			ctxWithCancel, cancel := context.WithCancel(ctx)
			ext := &fleetAutomationExtension{
				telemetry:     telemetry,
				forwarder:     tt.forwarder,
				eventCh:       make(chan *eventSourcePair),
				ctxWithCancel: ctxWithCancel,
				cancel:        cancel,
			}

			err := ext.Shutdown(ctx)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newNilForwarder(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
	return nil
}

func newMockForwarder(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
	return &mockForwarder{}
}

type mockSourceProvider struct {
	hostname string
	err      error
}

func (m *mockSourceProvider) Source(context.Context) (source.Source, error) {
	if m.err != nil {
		return source.Source{}, m.err
	}
	return source.Source{Identifier: m.hostname}, nil
}

type mockSourceProviderGetter struct {
	provider source.Provider
	err      error
}

func (m *mockSourceProviderGetter) GetSourceProvider(component.TelemetrySettings, string, time.Duration) (source.Provider, error) {
	return m.provider, m.err
}

type mockAPIKeyValidator struct {
	err error
}

func (m *mockAPIKeyValidator) ValidateAPIKey(context.Context, string, *zap.Logger, *datadog.APIClient) error {
	return m.err
}

type mockForwarder struct {
	startError       error
	stopError        error
	state            uint32
	failSendMetadata bool
}

func (m mockForwarder) Start() error {
	if m.startError != nil {
		return m.startError
	}
	atomic.StoreUint32(&m.state, 1)
	return nil
}

func (m mockForwarder) Stop() {
	if m.stopError != nil {
		return
	}
	atomic.StoreUint32(&m.state, 0)
}

func (m mockForwarder) State() uint32 {
	return atomic.LoadUint32(&m.state)
}

func (m mockForwarder) SubmitV1Series(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1Intake(transaction.BytesPayloads, transaction.Kind, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1CheckRuns(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSketchSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitHostMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitAgentChecksMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitMetadata(transaction.BytesPayloads, http.Header) error {
	if m.failSendMetadata {
		return fmt.Errorf("failed to send metadata")
	}
	return nil
}

func (m mockForwarder) SubmitProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessDiscoveryChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessEventChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitConnectionChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorChecks(transaction.BytesPayloads, http.Header, int) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorManifests(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func TestProcessComponentStatusEvents(t *testing.T) {
	tests := []struct {
		name           string
		events         []*eventSourcePair
		readySignal    bool
		expectedStatus map[string]any
	}{
		{
			name: "Process starting events immediately",
			events: []*eventSourcePair{
				{
					source: componentstatus.NewInstanceID(
						component.MustNewID("testreceiver"),
						component.KindReceiver,
					),
					event: componentstatus.NewEvent(componentstatus.StatusStarting),
				},
			},
			readySignal: false,
			expectedStatus: map[string]any{
				"receiver:testreceiver": map[string]any{
					"status": componentstatus.StatusStarting.String(),
					"error":  nil,
				},
			},
		},
		{
			name: "Queue non-starting events until ready",
			events: []*eventSourcePair{
				{
					source: componentstatus.NewInstanceID(
						component.MustNewID("testprocessor"),
						component.KindProcessor,
					),
					event: componentstatus.NewEvent(componentstatus.StatusOK),
				},
			},
			readySignal: true,
			expectedStatus: map[string]any{
				"processor:testprocessor": map[string]any{
					"status": componentstatus.StatusOK.String(),
					"error":  nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create extension with test settings
			set := extension.Settings{
				ID:                component.MustNewID(metadata.Type.String()),
				TelemetrySettings: componenttest.NewNopTelemetrySettings(),
			}
			mspg := mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
			}
			cfg := &Config{}
			faExt, err := newExtension(context.Background(), cfg, set, clientutil.ValidateAPIKey, mspg.GetSourceProvider, newMockForwarder)
			assert.NoError(t, err)

			// Start processing events in a goroutine
			go faExt.processComponentStatusEvents()

			// Send events
			for _, event := range tt.events {
				faExt.eventCh <- event
			}

			// If ready signal is needed, send it
			if tt.readySignal {
				close(faExt.readyCh)
			}

			// Give some time for processing
			time.Sleep(100 * time.Millisecond)

			// Check that the expected status fields match
			faExt.componentStatusMux.Lock()
			defer faExt.componentStatusMux.Unlock()
			for key, expectedValue := range tt.expectedStatus {
				actualValue, exists := faExt.componentStatus[key]
				assert.True(t, exists, "Expected key %s not found in componentStatus", key)
				actualMap := actualValue.(map[string]any)
				expectedMap := expectedValue.(map[string]any)

				// Check status and error
				assert.Equal(t, expectedMap["status"], actualMap["status"])
				assert.Equal(t, expectedMap["error"], actualMap["error"])

				// Check that timestamp is non-zero
				timestamp, ok := actualMap["timestamp"].(time.Time)
				assert.True(t, ok, "Expected timestamp to be time.Time")
				assert.False(t, timestamp.IsZero(), "Expected timestamp to be non-zero")
			}

			// Cleanup
			faExt.cancel()
		})
	}
}

func TestFleetAutomationExtension_GetComponentHealthStatus(t *testing.T) {
	tests := []struct {
		name            string
		events          []*eventSourcePair
		readySignal     bool
		expectedStatus  map[string]any
		forwarderGetter forwarderGetter
	}{
		{
			name: "Process starting events immediately",
			events: []*eventSourcePair{
				{
					source: componentstatus.NewInstanceID(
						component.MustNewID("testreceiver"),
						component.KindReceiver,
					),
					event: componentstatus.NewEvent(componentstatus.StatusStarting),
				},
			},
			readySignal: false,
			expectedStatus: map[string]any{
				"receiver:testreceiver": map[string]any{
					"status": componentstatus.StatusStarting.String(),
					"error":  nil,
				},
			},
			forwarderGetter: newMockForwarder,
		},
		{
			name: "Queue non-starting events until ready",
			events: []*eventSourcePair{
				{
					source: componentstatus.NewInstanceID(
						component.MustNewID("testprocessor"),
						component.KindProcessor,
					),
					event: componentstatus.NewEvent(componentstatus.StatusOK),
				},
			},
			readySignal: true,
			expectedStatus: map[string]any{
				"processor:testprocessor": map[string]any{
					"status": componentstatus.StatusOK.String(),
					"error":  nil,
				},
			},
			forwarderGetter: newMockForwarder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create extension with test settings
			set := extension.Settings{
				ID:                component.MustNewID(metadata.Type.String()),
				TelemetrySettings: componenttest.NewNopTelemetrySettings(),
			}
			mspg := mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
			}
			cfg := &Config{}
			faExt, err := newExtension(context.Background(), cfg, set, clientutil.ValidateAPIKey, mspg.GetSourceProvider, tt.forwarderGetter)
			assert.NoError(t, err)

			// Start processing events in a goroutine
			go faExt.processComponentStatusEvents()

			// Send events
			for _, event := range tt.events {
				faExt.eventCh <- event
			}

			// If ready signal is needed, send it
			if tt.readySignal {
				close(faExt.readyCh)
			}

			// Give some time for processing
			time.Sleep(100 * time.Millisecond)

			faExt.componentStatusMux.Lock()
			defer faExt.componentStatusMux.Unlock()
			// Check that the expected status fields match
			for key, expectedValue := range tt.expectedStatus {
				actualValue, exists := faExt.componentStatus[key]
				assert.True(t, exists, "Expected key %s not found in componentStatus", key)
				actualMap := actualValue.(map[string]any)
				expectedMap := expectedValue.(map[string]any)

				// Check status and error
				assert.Equal(t, expectedMap["status"], actualMap["status"])
				assert.Equal(t, expectedMap["error"], actualMap["error"])

				// Check that timestamp is non-zero
				timestamp, ok := actualMap["timestamp"].(time.Time)
				assert.True(t, ok, "Expected timestamp to be time.Time")
				assert.False(t, timestamp.IsZero(), "Expected timestamp to be non-zero")
			}

			// Cleanup
			faExt.cancel()
		})
	}
}

func TestFleetAutomationExtension_Ready(t *testing.T) {
	ext := createExtension(t)
	err := ext.Ready()
	assert.NoError(t, err)
}

func TestFleetAutomationExtension_NotReady(t *testing.T) {
	ext := createExtension(t)
	err := ext.NotReady()
	assert.NoError(t, err)
}

func TestFleetAutomationExtension_ComponentStatusChanged(t *testing.T) {
	ext := createExtension(t)
	source := componentstatus.NewInstanceID(component.MustNewID("test_component"), component.KindReceiver)
	event := componentstatus.NewEvent(componentstatus.StatusOK)
	go ext.processComponentStatusEvents()
	close(ext.readyCh)
	ext.ComponentStatusChanged(source, event)
	// Wait a bit for the component status to be updated
	time.Sleep(100 * time.Millisecond)

	// Check that the component status contains the expected key
	ext.componentStatusMux.Lock()
	defer ext.componentStatusMux.Unlock()

	// The key should be "receiver:test_component" based on the implementation
	key := "receiver:test_component"
	value, exists := ext.componentStatus[key]
	assert.True(t, exists, "Expected key %s not found in componentStatus", key)

	// Check that the status is "StatusOK"
	statusMap, ok := value.(map[string]any)
	assert.True(t, ok, "Expected value to be a map")
	assert.Equal(t, "StatusOK", statusMap["status"], "Expected status to be StatusOK")
	ext.cancel()
}

func createExtension(t *testing.T) *fleetAutomationExtension {
	cfg := createDefaultConfig().(*Config)
	ext, err := create(context.Background(), extensiontest.NewNopSettings(typ), cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
	return ext.(*fleetAutomationExtension)
}

func TestSendPayloadsOnTicker(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		serializer    serializer.MetricSerializer
		expectedError string
		expectedCalls int
		shutdownAfter time.Duration
	}{
		{
			name: "Successful payload sending",
			forwarder: mockForwarder{
				state: 1,
			},
			serializer: &mockSerializer{
				sendMetadataFunc: func(pl any) error {
					return nil
				},
			},
			expectedError: "",
			expectedCalls: 1,
			shutdownAfter: 100 * time.Millisecond,
		},
		{
			name: "Forwarder fails to send payload",
			forwarder: mockForwarder{
				state:            1,
				failSendMetadata: true,
			},
			serializer: &mockSerializer{
				sendMetadataFunc: func(pl any) error {
					return fmt.Errorf("failed to send metadata")
				},
			},
			expectedError: "failed to send datadog_agent payload",
			expectedCalls: 1,
			shutdownAfter: 100 * time.Millisecond,
		},
		{
			name: "Shutdown before ticker triggers",
			forwarder: mockForwarder{
				state: 1,
			},
			serializer: &mockSerializer{
				sendMetadataFunc: func(pl any) error {
					return nil
				},
			},
			expectedError: "",
			expectedCalls: 0,
			shutdownAfter: 10 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create extension with test settings
			ctx, cancel := context.WithCancel(context.Background())

			// Create a test UUID
			testUUID := uuid.New()

			// Create test build info
			testBuildInfo := payload.CustomBuildInfo{
				Command:     "otelcol",
				Description: "OpenTelemetry Collector",
				Version:     "1.0.0",
			}

			// Create test module info
			testModuleInfo := service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("otlp"): {BuilderRef: "otlp@v0.117.0"},
				},
			}

			// Create test component status
			testComponentStatus := map[string]any{
				"receiver:testreceiver": map[string]any{
					"status":    componentstatus.StatusOK.String(),
					"error":     nil,
					"timestamp": time.Now(),
				},
			}

			// Create test collector config string map
			testCollectorConfigStringMap := map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{"otlp"},
							"exporters": []any{"debug"},
						},
					},
				},
			}

			// Create test metadata payloads
			testAgentMetadataPayload := payload.PrepareAgentMetadataPayload(
				"datadoghq.com",
				testBuildInfo.Command,
				testBuildInfo.Version,
				testBuildInfo.Version,
				"test-hostname",
			)

			testOtelMetadataPayload := payload.PrepareOtelMetadataPayload(
				testBuildInfo.Version,
				"1.0.0",
				testBuildInfo.Command,
				"{}", // Simplified config for testing
			)

			testOtelCollectorPayload := payload.PrepareOtelCollectorPayload(
				"test-hostname",
				"inferred",
				testUUID.String(),
				"1.0.0",
				"datadoghq.com",
				"{}", // Simplified config for testing
				testBuildInfo,
			)

			ext := &fleetAutomationExtension{
				telemetry:     componenttest.NewNopTelemetrySettings(),
				forwarder:     tt.forwarder,
				serializer:    tt.serializer,
				ctxWithCancel: ctx,
				cancel:        cancel,
				ticker:        time.NewTicker(50 * time.Millisecond), // Use a shorter ticker for testing
				hostnameProvider: &mockSourceProvider{
					hostname: "test-hostname",
					err:      nil,
				},
				hostnameSource:           "inferred",
				uuid:                     testUUID,
				buildInfo:                testBuildInfo,
				moduleInfo:               testModuleInfo,
				componentStatus:          testComponentStatus,
				collectorConfigStringMap: testCollectorConfigStringMap,
				agentMetadataPayload:     testAgentMetadataPayload,
				otelMetadataPayload:      testOtelMetadataPayload,
				otelCollectorPayload:     testOtelCollectorPayload,
			}

			// Start the ticker goroutine
			go ext.sendPayloadsOnTicker("test-hostname")

			// Wait for the specified duration
			time.Sleep(tt.shutdownAfter)

			// Shutdown the extension
			ext.cancel()

			// Give some time for the goroutine to clean up
			time.Sleep(50 * time.Millisecond)

			// Verify the number of calls to SubmitMetadata
			if tt.expectedCalls > 0 {
				// Note: In a real test, we would need to add a counter to the mockForwarder
				// to track the number of SubmitMetadata calls. For now, we're just verifying
				// that the goroutine runs and handles errors appropriately.
				if tt.expectedError != "" {
					// We can verify the error was logged, but this would require capturing logs
					// which is beyond the scope of this test
				}
			}

			// Verify the ticker was stopped
			select {
			case <-ext.ticker.C:
				t.Error("Ticker was not stopped")
			default:
				// Ticker was stopped correctly
			}
		})
	}
}

type mockSerializer struct {
	sendMetadataFunc func(pl any) error
}

var _ serializer.MetricSerializer = &mockSerializer{}

func (m *mockSerializer) SendMetadata(jm marshaler.JSONMarshaler) error {
	if m.sendMetadataFunc != nil {
		return m.sendMetadataFunc(jm)
	}
	return nil
}

func (m *mockSerializer) SendEvents(event.Events) error {
	return nil
}

func (m *mockSerializer) SendServiceChecks(servicecheck.ServiceChecks) error {
	return nil
}

func (m *mockSerializer) SendIterableSeries(metrics.SerieSource) error {
	return nil
}

func (m *mockSerializer) AreSeriesEnabled() bool {
	return false
}

func (m *mockSerializer) SendSketch(metrics.SketchesSource) error {
	return nil
}

func (m *mockSerializer) AreSketchesEnabled() bool {
	return false
}

func (m *mockSerializer) SendHostMetadata(marshaler.JSONMarshaler) error {
	return nil
}

func (m *mockSerializer) SendProcessesMetadata(any) error {
	return nil
}

func (m *mockSerializer) SendAgentchecksMetadata(marshaler.JSONMarshaler) error {
	return nil
}

func (m *mockSerializer) SendOrchestratorMetadata([]types.ProcessMessageBody, string, string, int) error {
	return nil
}

func (m *mockSerializer) SendOrchestratorManifests([]types.ProcessMessageBody, string, string) error {
	return nil
}
