// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	emptyFullComponents = `{
  "full_components": []
}`
	envVarStatusOK = `{
  "status": "ok"
}`
)

type mockSerializer struct {
	sendMetadataFunc func(pl any) error
}

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

type mockForwarder struct {
	state uint32
}

func (m *mockForwarder) Start() error {
	return nil
}

func (m *mockForwarder) State() uint32 {
	return m.state
}

func (m *mockForwarder) Stop() {
}

func (m *mockForwarder) SubmitV1Series(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitV1Intake(transaction.BytesPayloads, transaction.Kind, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitV1CheckRuns(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitSketchSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitHostMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitAgentChecksMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m *mockForwarder) SubmitProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitProcessDiscoveryChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitProcessEventChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitRTProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitRTContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitConnectionChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitOrchestratorChecks(transaction.BytesPayloads, http.Header, int) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m *mockForwarder) SubmitOrchestratorManifests(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

type invalidForwarder struct {
	defaultforwarder.Forwarder
}

// Add this type before TestHandleMetadata
type failingMarshaler struct {
	payload.CombinedPayload
}

func (f *failingMarshaler) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("failed to marshal JSON")
}

func TestServerStart(t *testing.T) {
	tests := []struct {
		name         string
		setupServer  func() (*Server, *observer.ObservedLogs)
		expectedLogs []string
	}{
		{
			name: "Start server successfully",
			setupServer: func() (*Server, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				s := NewServer(logger, &mockSerializer{}, &mockForwarder{}, &Config{
					ServerConfig: confighttp.ServerConfig{
						Endpoint: testutil.EndpointForPort(DefaultServerPort),
					},
					Enabled: true,
				})
				return s, logs
			},
			expectedLogs: []string{"HTTP Server started on port 8088"},
		},
		{
			name: "Server disabled via configuration",
			setupServer: func() (*Server, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				s := NewServer(logger, &mockSerializer{}, &mockForwarder{}, &Config{
					Enabled: false,
				})
				return s, logs
			},
			expectedLogs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, logs := tt.setupServer()
			s.Start(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Verify the logs
			for _, expectedLog := range tt.expectedLogs {
				found := false
				for _, log := range logs.All() {
					if log.Message == expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", expectedLog)
			}

			// Stop the server
			s.Stop()
		})
	}
}

func TestPrepareAndSendFleetAutomationPayloads(t *testing.T) {
	tests := []struct {
		name                   string
		setupTest              func() (*zap.Logger, *observer.ObservedLogs, serializer.MetricSerializer, defaultForwarderInterface)
		expectedError          string
		expectedLogs           []string
		expectedEnvironmentVar string
		expectedProvidedConfig string
		serverResponseCode     int
		serverResponse         string
	}{
		{
			name: "Successful payload preparation and sending",
			setupTest: func() (*zap.Logger, *observer.ObservedLogs, serializer.MetricSerializer, defaultForwarderInterface) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return logger, logs, &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					}, &mockForwarder{
						state: 1,
					}
			},
			expectedError:          "",
			expectedLogs:           []string{},
			expectedEnvironmentVar: envVarStatusOK,
			expectedProvidedConfig: emptyFullComponents,
			serverResponseCode:     http.StatusOK,
			serverResponse:         `{"status": "ok"}`,
		},
		{
			name: "Failed to get health check status",
			setupTest: func() (*zap.Logger, *observer.ObservedLogs, serializer.MetricSerializer, defaultForwarderInterface) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return logger, logs, &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					}, &mockForwarder{
						state: defaultforwarder.Started,
					}
			},
			expectedError:          "",
			expectedLogs:           []string{},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
			serverResponseCode:     http.StatusInternalServerError,
			serverResponse:         `Internal Server Error`,
		},
		{
			name: "Failed to send datadog_agent payload",
			setupTest: func() (*zap.Logger, *observer.ObservedLogs, serializer.MetricSerializer, defaultForwarderInterface) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return logger, logs, &mockSerializer{
						sendMetadataFunc: func(pl any) error {
							if _, ok := pl.(*payload.AgentPayload); ok {
								return errors.New("failed to send payload")
							}
							return nil
						},
					}, &mockForwarder{
						state: defaultforwarder.Started,
					}
			},
			expectedError:          "failed to send datadog_agent payload: failed to send payload",
			expectedLogs:           []string{},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
			serverResponseCode:     http.StatusInternalServerError,
			serverResponse:         `Internal Server Error`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs, serializer, forwarder := tt.setupTest()

			componentStatus := map[string]any{
				"status": "ok",
			}

			combinedPayload, err := PrepareAndSendFleetAutomationPayloads(
				logger,
				serializer,
				forwarder,
				"test-hostname",
				"test-uuid",
				componentStatus,
				service.ModuleInfos{},
				map[string]any{},
				payload.AgentMetadata{},
				payload.OtelMetadata{},
				payload.OtelCollector{},
			)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err.Error())
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, combinedPayload)

			// Verify the logs
			for _, expectedLog := range tt.expectedLogs {
				found := false
				for _, log := range logs.All() {
					if log.Message == expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", expectedLog)
			}
		})
	}
}

const successfulInstanceResponse = `{
  "collector_payload": {
    "hostname": "test-hostname",
    "timestamp": 1741003200000000000,
    "otel_collector": {
      "host_key": "",
      "hostname": "",
      "hostname_source": "",
      "collector_id": "",
      "collector_version": "",
      "config_site": "",
      "api_key_uuid": "",
      "full_components": [],
      "active_components": null,
      "build_info": {
        "command": "",
        "description": "",
        "version": ""
      },
      "full_configuration": "",
      "health_status": "{}"
    },
    "uuid": "test-uuid"
  },
  "otel_payload": {
    "hostname": "test-hostname",
    "timestamp": 1741003200000000000,
    "otel_metadata": {
      "command": "",
      "description": "",
      "enabled": false,
      "environment_variable_configuration": "{}",
      "extension_version": "",
      "full_configuration": "",
      "provided_configuration": "{\n  \"full_components\": []\n}",
      "runtime_override_configuration": "",
      "version": ""
    },
    "uuid": "test-uuid"
  },
  "agent_payload": {
    "hostname": "test-hostname",
    "timestamp": 1741003200000000000,
    "agent_metadata": {
      "flavor": ""
    },
    "uuid": "test-uuid"
  }
}`

func TestHandleMetadata(t *testing.T) {
	mockTime := time.Date(2025, time.March, 3, 12, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time {
		return mockTime
	}
	defer func() {
		nowFunc = time.Now
	}()
	tests := []struct {
		name           string
		setupTest      func() (*zap.Logger, serializer.MetricSerializer, defaultForwarderInterface)
		hostnameSource string
		expectedCode   int
		expectedBody   string
	}{
		{
			name: "Successful metadata handling",
			setupTest: func() (*zap.Logger, serializer.MetricSerializer, defaultForwarderInterface) {
				core, _ := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return logger, &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					}, &mockForwarder{
						state: defaultforwarder.Started,
					}
			},
			hostnameSource: "config",
			expectedCode:   http.StatusOK,
			expectedBody:   successfulInstanceResponse,
		},
		{
			name: "Failed metadata handling - serializer error",
			setupTest: func() (*zap.Logger, serializer.MetricSerializer, defaultForwarderInterface) {
				core, _ := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return logger, &mockSerializer{
						sendMetadataFunc: func(any) error {
							return fmt.Errorf("failed to send metadata")
						},
					}, &mockForwarder{
						state: defaultforwarder.Started,
					}
			},
			hostnameSource: "config",
			expectedCode:   http.StatusInternalServerError,
			expectedBody:   "Failed to prepare and send fleet automation payloads\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, serializer, forwarder := tt.setupTest()

			w := httptest.NewRecorder()

			HandleMetadata(
				w,
				logger,
				tt.hostnameSource,
				"test-hostname",
				"test-uuid",
				&map[string]any{},
				&service.ModuleInfos{},
				&map[string]any{},
				&payload.AgentMetadata{},
				&payload.OtelMetadata{},
				&payload.OtelCollector{},
				serializer,
				forwarder,
			)

			assert.Equal(t, tt.expectedCode, w.Code)
			assert.Equal(t, tt.expectedBody, w.Body.String())
		})
	}
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name       string
		logger     *zap.Logger
		serializer serializer.MetricSerializer
		forwarder  defaultforwarder.Forwarder
		config     *Config
		want       *Server
	}{
		{
			name:       "valid forwarder",
			logger:     zap.NewNop(),
			serializer: &mockSerializer{},
			forwarder:  &mockForwarder{},
			config:     &Config{},
			want: &Server{
				logger:     zap.NewNop(),
				serializer: &mockSerializer{},
				forwarder:  &mockForwarder{},
				config:     &Config{},
			},
		},
		{
			name:       "invalid forwarder",
			logger:     zap.NewNop(),
			serializer: &mockSerializer{},
			forwarder:  &invalidForwarder{},
			config:     &Config{},
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewServer(tt.logger, tt.serializer, tt.forwarder, tt.config)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tt.want.logger, got.logger)
			assert.Equal(t, tt.want.serializer, got.serializer)
			assert.Equal(t, tt.want.forwarder, got.forwarder)
			assert.Equal(t, tt.want.config, got.config)
		})
	}
}

func TestServer_Start(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "server starts successfully",
			config: &Config{
				Enabled: true,
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
		{
			name: "server disabled via config",
			config: &Config{
				Enabled: false,
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				logger:     zap.NewNop(),
				serializer: &mockSerializer{},
				forwarder:  &mockForwarder{},
				config:     tt.config,
			}

			s.Start(tt.handler)

			if tt.config.Enabled {
				// Give the server a moment to start
				time.Sleep(100 * time.Millisecond)

				// Test the server is running
				resp, err := http.Get("http://localhost:8088/metadata")
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				resp.Body.Close()
			}

			s.Stop()
		})
	}
}
