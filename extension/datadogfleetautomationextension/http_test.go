package datadogfleetautomationextension

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
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
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				return &fleetAutomationExtension{
					telemetry: component.TelemetrySettings{
						Logger: logger,
					},
					ticker: time.NewTicker(20 * time.Minute),
					done:   make(chan bool),
					httpServer: &http.Server{
						Addr:    server.Listener.Addr().String(),
						Handler: server.Config.Handler,
					},
				}, logs
			},
			expectedLogs: []string{"HTTP Server started on port 8088"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, logs := tt.setupExtension()
			err := e.startLocalConfigServer()
			assert.NoError(t, err)

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
			if e.httpServer != nil {
				e.httpServer.Close()
			}
		})
	}
}

func TestGetHealthCheckStatus(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		serverStatus   int
		expectedResult map[string]any
		expectedError  string
		url            string
	}{
		{
			name:           "Successful response",
			serverResponse: `{"status": "ok"}`,
			serverStatus:   http.StatusOK,
			expectedResult: map[string]any{"status": "ok"},
			expectedError:  "",
			url:            "",
		},
		{
			name:           "Server error",
			serverResponse: `Internal Server Error`,
			serverStatus:   http.StatusInternalServerError,
			expectedResult: nil,
			expectedError:  "failed to make request to health check endpoint",
			url:            "invalid url",
		},
		{
			name:           "Invalid JSON response",
			serverResponse: `Invalid JSON`,
			serverStatus:   http.StatusOK,
			expectedResult: nil,
			expectedError:  "failed to decode JSON response",
			url:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
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
							"enabled": true,
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
	sendMetadataFunc func(payload interface{}) error
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

// func TestPrepareAndSendFleetAutomationPayloads(t *testing.T) {
// 	tests := []struct {
// 		name                   string
// 		setupExtension         func() (*fleetAutomationExtension, *observer.ObservedLogs)
// 		expectedError          string
// 		expectedLogs           []string
// 		expectedEnvironmentVar string
// 		expectedProvidedConfig string
// 	}{
// 		{
// 			name: "Successful payload preparation and sending",
// 			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {

// 				serverResponse := `{"status": "ok"}`
// 				serverStatus := http.StatusOK
// 				core, logs := observer.New(zapcore.InfoLevel)
// 				logger := zap.New(core)
// 				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 					w.WriteHeader(serverStatus)
// 					w.Write([]byte(serverResponse))
// 				}))
// 				defer server.Close()
// 				return &fleetAutomationExtension{
// 					telemetry: component.TelemetrySettings{
// 						Logger: logger,
// 					},
// 					healthCheckV2Enabled: true,
// 					healthCheckV2Config: map[string]any{
// 						"http": map[string]any{
// 							"endpoint": server.URL,
// 							"status": map[string]any{
// 								"enabled": true,
// 								"path":    "/health/status",
// 							},
// 						},
// 					},
// 					forwarder: &mockForwarder{
// 						state: 1,
// 					},
// 					serializer: &mockSerializer{
// 						sendMetadataFunc: func(payload interface{}) error {
// 							return nil
// 						},
// 					},
// 					hostname: "test-hostname",
// 					ticker:   time.NewTicker(20 * time.Minute),
// 					done:     make(chan bool),
// 				}, logs
// 			},
// 			expectedError: "",
// 			expectedLogs:  []string{},
// 			expectedEnvironmentVar: `{
//   "status": "ok"
// }`,
// 			expectedProvidedConfig: `{
//   "full_components": []
// }`,
// 		},
// 		{
// 			name: "Failed to get health check status",
// 			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
// 				core, logs := observer.New(zapcore.InfoLevel)
// 				logger := zap.New(core)

// 				return &fleetAutomationExtension{
// 					telemetry: component.TelemetrySettings{
// 						Logger: logger,
// 					},
// 					healthCheckV2Enabled: true,
// 					healthCheckV2Config: map[string]any{
// 						"http": map[string]any{
// 							"endpoint": "http://localhost:13133",
// 							"status": map[string]any{
// 								"enabled": true,
// 								"path":    "/health/status",
// 							},
// 						},
// 					},
// 					forwarder: &mockForwarder{
// 						state: defaultforwarder.Started,
// 					},
// 					serializer: &mockSerializer{
// 						sendMetadataFunc: func(payload interface{}) error {
// 							return nil
// 						},
// 					},
// 					hostname: "test-hostname",
// 					ticker:   time.NewTicker(20 * time.Minute),
// 					done:     make(chan bool),
// 				}, logs
// 			},
// 			expectedError:          "",
// 			expectedLogs:           []string{"Failed to get health check status"},
// 			expectedEnvironmentVar: "",
// 			expectedProvidedConfig: `{
//   "full_components": []
// }`,
// 		},
// 		{
// 			name: "Failed to send datadog_agent payload",
// 			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
// 				core, logs := observer.New(zapcore.InfoLevel)
// 				logger := zap.New(core)

// 				return &fleetAutomationExtension{
// 					telemetry: component.TelemetrySettings{
// 						Logger: logger,
// 					},
// 					healthCheckV2Enabled: false,
// 					forwarder: &mockForwarder{
// 						state: defaultforwarder.Started,
// 					},
// 					serializer: &mockSerializer{
// 						sendMetadataFunc: func(payload interface{}) error {
// 							if _, ok := payload.(*agentPayload); ok {
// 								return errors.New("failed to send datadog_agent payload")
// 							}
// 							return nil
// 						},
// 					},
// 					hostname: "test-hostname",
// 					ticker:   time.NewTicker(20 * time.Minute),
// 					done:     make(chan bool),
// 				}, logs
// 			},
// 			expectedError:          "failed to send datadog_agent payload: failed to send datadog_agent payload",
// 			expectedLogs:           []string{},
// 			expectedEnvironmentVar: "",
// 			expectedProvidedConfig: `{
//   "full_components": []
// }`,
// 		},
// 		{
// 			name: "Forwarder not started",
// 			setupExtension: func() (*fleetAutomationExtension, *observer.ObservedLogs) {
// 				core, logs := observer.New(zapcore.InfoLevel)
// 				logger := zap.New(core)

// 				return &fleetAutomationExtension{
// 					telemetry: component.TelemetrySettings{
// 						Logger: logger,
// 					},
// 					healthCheckV2Enabled: false,
// 					forwarder: &mockForwarder{
// 						state: defaultforwarder.Stopped,
// 					},
// 					serializer: &mockSerializer{},
// 					hostname:   "test-hostname",
// 					ticker:     time.NewTicker(20 * time.Minute),
// 					done:       make(chan bool),
// 				}, logs
// 			},
// 			expectedError:          "",
// 			expectedLogs:           []string{"Forwarder is not started, skipping sending payloads"},
// 			expectedEnvironmentVar: "",
// 			expectedProvidedConfig: `{
//   "full_components": []
// }`,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			e, logs := tt.setupExtension()
// 			var serverResponse string
// 			var serverStatus int
// 			if tt.expectedEnvironmentVar != "" {
// 				serverResponse = `{"status": "ok"}`
// 				serverStatus = http.StatusOK
// 			} else {
// 				serverResponse = "{}"
// 				serverStatus = http.StatusAccepted
// 			}
// 			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				w.WriteHeader(serverStatus)
// 				w.Write([]byte(serverResponse))
// 			}))
// 			defer server.Close()
// 			e.healthCheckV2Config["http"].(map[string]any)["endpoint"] = server.URL
// 			// Call prepareAndSendFleetAutomationPayloads
// 			payload, err := e.prepareAndSendFleetAutomationPayloads()

// 			// Verify the result
// 			if tt.expectedError == "" {
// 				assert.NoError(t, err)
// 			} else {
// 				assert.Error(t, err)
// 				assert.Contains(t, err.Error(), tt.expectedError)
// 			}

// 			// Verify the logs
// 			for _, expectedLog := range tt.expectedLogs {
// 				found := false
// 				for _, log := range logs.All() {
// 					if log.Message == expectedLog {
// 						found = true
// 						break
// 					}
// 				}
// 				assert.True(t, found, "Expected log message not found: %s", expectedLog)
// 			}

// 			// Verify the environment variable configuration
// 			if tt.expectedEnvironmentVar != "" {
// 				assert.Equal(t, tt.expectedEnvironmentVar, e.otelMetadataPayload.EnvironmentVariableConfiguration)
// 			}

// 			// Verify the provided configuration
// 			if tt.expectedProvidedConfig != "" {
// 				assert.Contains(t, e.otelMetadataPayload.ProvidedConfiguration, tt.expectedProvidedConfig)
// 			}

// 			// Verify the payload
// 			if tt.expectedError == "" {
// 				assert.NotNil(t, payload)
// 				assert.Equal(t, e.hostname, payload.AgentPayload.Hostname)
// 				assert.Equal(t, e.hostname, payload.OtelPayload.Hostname)
// 			} else {
// 				assert.Nil(t, payload)
// 			}
// 		})
// 	}
// }
