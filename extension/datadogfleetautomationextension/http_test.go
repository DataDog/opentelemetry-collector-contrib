// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	googleuuid "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
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

func TestStartLocalConfigServer(t *testing.T) {
	tests := []struct {
		name           string
		setupExtension func() (*fleetAutomationExtension, *observer.ObservedLogs)
		expectedLogs   []string
	}{
		{
			name: "Start server successfully",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					ticker: time.NewTicker(defaultReporterPeriod),
					done:   make(chan bool),
					httpServer: &http.Server{
						Addr:         server.Listener.Addr().String(),
						Handler:      server.Config.Handler,
						ReadTimeout:  5 * time.Second,  // Set read timeout to 5 seconds
						WriteTimeout: 10 * time.Second, // Set write timeout to 10 seconds
					},
				}, logs
			},
			expectedLogs: []string{"HTTP Server started on port 8088"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, logs := tt.setupExtension()
			e.startLocalConfigServer()

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
			e.stopLocalConfigServer()
		})
	}
}

func TestGetHealthCheckStatus(t *testing.T) {
	tests := []struct {
		name               string
		serverResponse     string
		serverStatus       int
		expectedResult     map[string]any
		expectedError      string
		url                string
		healthCheckEnabled bool
	}{
		{
			name:               "Successful response",
			serverResponse:     `{"status": "ok"}`,
			serverStatus:       http.StatusOK,
			expectedResult:     map[string]any{"status": "ok"},
			expectedError:      "",
			url:                "",
			healthCheckEnabled: true,
		},
		{
			name:               "invalid url",
			serverResponse:     `Internal Server Error`,
			serverStatus:       http.StatusInternalServerError,
			expectedResult:     nil,
			expectedError:      "invalid URL:",
			url:                "invalid url",
			healthCheckEnabled: true,
		},
		{
			name:               "Invalid JSON response",
			serverResponse:     `Invalid JSON`,
			serverStatus:       http.StatusOK,
			expectedResult:     nil,
			expectedError:      "failed to decode JSON response",
			url:                "",
			healthCheckEnabled: true,
		},
		{
			name:               "Health check not enabled",
			serverResponse:     `{}`,
			serverStatus:       http.StatusOK,
			expectedResult:     nil,
			expectedError:      "http health check v2 extension is not enabled",
			url:                "",
			healthCheckEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.serverStatus)
				_, err := w.Write([]byte(tt.serverResponse))
				assert.NoError(t, err)
			}))
			defer server.Close()
			if tt.url == "" {
				tt.url = server.URL
			}
			// Create a fleetAutomationExtension instance
			logger := zaptest.NewLogger(t)
			e := &fleetAutomationExtension{
				telemetry: component.TelemetrySettings{
					Logger: logger,
				},
				healthCheckV2Config: map[string]any{
					"http": map[string]any{
						"endpoint": tt.url,
						"status": map[string]any{
							"enabled": tt.healthCheckEnabled,
							"path":    "/health/status",
						},
					},
				},
			}

			// Call getHealthCheckStatus
			result, err := e.getHealthCheckStatus()

			// Verify the result
			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

type mockSerializer struct {
	sendMetadataFunc func(payload any) error
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

func TestPrepareAndSendFleetAutomationPayloads(t *testing.T) {
	tests := []struct {
		name                   string
		setupExtension         func() (*fleetAutomationExtension, *observer.ObservedLogs)
		expectedError          string
		expectedLogs           []string
		expectedEnvironmentVar string
		expectedProvidedConfig string
		serverResponseCode     int
		serverResponse         string
	}{
		{
			name: "Successful payload preparation and sending",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)
				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					healthCheckV2Enabled: true,
					healthCheckV2Config: map[string]any{
						"http": map[string]any{
							"endpoint": "",
							"status": map[string]any{
								"enabled": true,
								"path":    "/health/status",
							},
						},
					},
					forwarder: &mockForwarder{
						state: 1,
					},
					serializer: &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					},
					hostname: "test-hostname",
					ticker:   time.NewTicker(defaultReporterPeriod),
					done:     make(chan bool),
				}, logs
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
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					healthCheckV2Enabled: true,
					healthCheckV2Config: map[string]any{
						"http": map[string]any{
							"endpoint": "http://localhost:13133",
							"status": map[string]any{
								"enabled": true,
								"path":    "/health/status",
							},
						},
					},
					forwarder: &mockForwarder{
						state: defaultforwarder.Started,
					},
					serializer: &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					},
					hostname: "test-hostname",
					ticker:   time.NewTicker(defaultReporterPeriod),
					done:     make(chan bool),
				}, logs
			},
			expectedError:          "",
			expectedLogs:           []string{"Failed to get health check status"},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
			serverResponseCode:     http.StatusInternalServerError,
			serverResponse:         `Internal Server Error`,
		},
		{
			name: "Failed to send datadog_agent payload",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					healthCheckV2Enabled: false,
					forwarder: &mockForwarder{
						state: defaultforwarder.Started,
					},
					serializer: &mockSerializer{
						sendMetadataFunc: func(payload any) error {
							if _, ok := payload.(*agentPayload); ok {
								return errors.New("failed to send payload")
							}
							return nil
						},
					},
					hostname: "test-hostname",
					ticker:   time.NewTicker(defaultReporterPeriod),
					done:     make(chan bool),
				}, logs
			},
			expectedError:          "failed to send datadog_agent payload: failed to send payload",
			expectedLogs:           []string{},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
		},
		{
			name: "Failed to send datadog_agent_otel payload",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					healthCheckV2Enabled: false,
					forwarder: &mockForwarder{
						state: defaultforwarder.Started,
					},
					serializer: &mockSerializer{
						sendMetadataFunc: func(payload any) error {
							if _, ok := payload.(*otelAgentPayload); ok {
								return errors.New("failed to send payload")
							}
							return nil
						},
					},
					hostname: "test-hostname",
					ticker:   time.NewTicker(defaultReporterPeriod),
					done:     make(chan bool),
				}, logs
			},
			expectedError:          "failed to send datadog_agent_otel payload: failed to send payload",
			expectedLogs:           []string{},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
		},
		{
			name: "Forwarder not started",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					healthCheckV2Enabled: false,
					forwarder: &mockForwarder{
						state: defaultforwarder.Stopped,
					},
					serializer: &mockSerializer{},
					hostname:   "test-hostname",
					ticker:     time.NewTicker(defaultReporterPeriod),
					done:       make(chan bool),
				}, logs
			},
			expectedError:          "",
			expectedLogs:           []string{"Forwarder is not started, skipping sending payloads"},
			expectedEnvironmentVar: "",
			expectedProvidedConfig: emptyFullComponents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, logs := tt.setupExtension()
			var serverResponse string
			var serverStatus int
			if tt.serverResponseCode != 0 {
				serverResponse = tt.serverResponse
				serverStatus = tt.serverResponseCode
			} else {
				serverResponse = "{}"
				serverStatus = http.StatusAccepted
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(serverStatus)
				_, err := w.Write([]byte(serverResponse))
				assert.NoError(t, err)
			}))
			defer server.Close()
			if e.healthCheckV2Enabled {
				e.healthCheckV2Config["http"].(map[string]any)["endpoint"] = server.URL
			}
			// Call prepareAndSendFleetAutomationPayloads
			payload, err := e.prepareAndSendFleetAutomationPayloads()

			// Verify the result
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}

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

			// Verify the environment variable configuration
			if tt.expectedEnvironmentVar != "" {
				assert.Equal(t, tt.expectedEnvironmentVar, e.otelMetadataPayload.EnvironmentVariableConfiguration)
			}

			// Verify the provided configuration
			if tt.expectedProvidedConfig != "" {
				assert.Contains(t, e.otelMetadataPayload.ProvidedConfiguration, tt.expectedProvidedConfig)
			}

			// Verify the payload
			if tt.expectedError == "" {
				assert.NotNil(t, payload)
				assert.Equal(t, e.hostname, payload.AgentPayload.Hostname)
				assert.Equal(t, e.hostname, payload.OtelPayload.Hostname)
			} else {
				assert.Nil(t, payload)
			}
			close(e.done)
		})
	}
}

const successfulInstanceResponse = `{
  "otel_payload": {
    "hostname": "test-hostname",
    "timestamp": 1741003200000000000,
    "otel_metadata": {
      "command": "",
      "description": "",
      "enabled": false,
      "environment_variable_configuration": "",
      "extension_version": "",
      "full_configuration": "",
      "provided_configuration": "{\n  \"full_components\": []\n}",
      "runtime_override_configuration": "",
      "version": ""
    },
    "uuid": "123e4567-e89b-12d3-a456-426614174000"
  },
  "agent_payload": {
    "hostname": "test-hostname",
    "timestamp": 1741003200000000000,
    "agent_metadata": {},
    "uuid": "123e4567-e89b-12d3-a456-426614174000"
  }
}`

func TestHandleMetadata(t *testing.T) {
	// Mock the current time
	mockTime := time.Date(2025, time.March, 3, 12, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time {
		return mockTime
	}
	defer func() {
		nowFunc = time.Now
	}()
	// Mock the UUID
	mockUUID := "123e4567-e89b-12d3-a456-426614174000"

	tests := []struct {
		name           string
		setupExtension func() (*fleetAutomationExtension, *observer.ObservedLogs)
		expectedStatus int
		expectedBody   string
		expectedLogs   []string
	}{
		{
			name: "Successful instance",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					hostname: "test-hostname",
					serializer: &mockSerializer{
						sendMetadataFunc: func(any) error {
							return nil
						},
					},
					ticker: time.NewTicker(defaultReporterPeriod),
					done:   make(chan bool),
					uuid:   googleuuid.MustParse(mockUUID),
				}, logs
			},
			expectedStatus: http.StatusOK,
			expectedBody:   successfulInstanceResponse,
			expectedLogs:   []string{},
		},
		{
			name: "Hostname is empty",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					hostnameSource: "unset",
					ticker:         time.NewTicker(defaultReporterPeriod),
					done:           make(chan bool),
					uuid:           googleuuid.MustParse(mockUUID),
				}, logs
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Fleet automation payloads not sent since the hostname is empty",
			expectedLogs:   []string{"Skipping fleet automation payloads since the hostname is empty"},
		},
		{
			name: "Failed to prepare and send fleet automation payloads",
			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
				core, logs := observer.New(zapcore.InfoLevel)
				logger := zap.New(core)

				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					hostname: "test-hostname",
					serializer: &mockSerializer{
						sendMetadataFunc: func(any) error {
							return errors.New("failed to send payload")
						},
					},
					ticker: time.NewTicker(defaultReporterPeriod),
					done:   make(chan bool),
					uuid:   googleuuid.MustParse(mockUUID),
				}, logs
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Failed to prepare and send fleet automation payloads\n",
			expectedLogs:   []string{"Failed to prepare and send fleet automation payloads"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, logs := tt.setupExtension()
			e.forwarder = &mockForwarder{
				state: defaultforwarder.Started,
			}

			// Create a test HTTP request and response recorder
			req := httptest.NewRequest(http.MethodGet, "/metadata", nil)
			rr := httptest.NewRecorder()

			// Call handleMetadata
			e.handleMetadata(rr, req)

			// Verify the response status code
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Verify the response body
			assert.Equal(t, tt.expectedBody, rr.Body.String())

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
			close(e.done)
		})
	}
}

func TestMakeGetRequest(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		serverStatus   int
		expectedError  string
		expectedBody   string
		url            string
	}{
		{
			name:           "Successful response",
			serverResponse: `{"status": "ok"}`,
			serverStatus:   http.StatusOK,
			expectedError:  "",
			expectedBody:   `{"status": "ok"}`,
		},
		{
			name:           "Server error",
			serverResponse: `Internal Server Error`,
			serverStatus:   http.StatusInternalServerError,
			expectedError:  "",
			expectedBody:   `Internal Server Error`,
		},
		{
			name:           "Invalid URL",
			serverResponse: ``,
			serverStatus:   http.StatusOK,
			expectedError:  "missing protocol scheme",
			expectedBody:   ``,
			url:            "://invalid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.serverStatus)
				_, err := w.Write([]byte(tt.serverResponse))
				assert.NoError(t, err)
			}))
			defer server.Close()

			// Determine the URL to use for the test
			if tt.url == "" {
				tt.url = server.URL
			}

			// Call makeGetRequest
			resp, err := makeGetRequest(tt.url)
			defer func() {
				if resp != nil {
					_, _ = resp.Body.Read(nil)
					_ = resp.Body.Close()
				}
			}()
			// Verify the result
			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				body := make([]byte, len(tt.expectedBody))
				_, err = resp.Body.Read(body)
				assert.ErrorContains(t, err, "EOF")
				assert.Equal(t, tt.expectedBody, string(body))
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, resp)
			}
		})
	}
}

func TestIsLocalAddress(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		{
			name:     "Localhost",
			hostname: "localhost",
			expected: true,
		},
		{
			name:     "IPv4 Loopback",
			hostname: "127.0.0.1",
			expected: true,
		},
		{
			name:     "IPv6 Loopback",
			hostname: "::1",
			expected: true,
		},
		{
			name:     "Non-local address",
			hostname: "192.168.1.1",
			expected: false,
		},
		{
			name:     "Invalid IP address",
			hostname: "invalid-ip",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLocalAddress(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}
