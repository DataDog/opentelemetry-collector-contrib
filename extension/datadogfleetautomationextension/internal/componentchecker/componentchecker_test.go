// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package componentchecker

import (
	"errors"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCheckComponentConfiguration(t *testing.T) {
	tests := []struct {
		name                     string
		collectorConfigStringMap map[string]any
		typ                      string
		componentsKind           string
		expectedConfigured       bool
		expectedTypeString       string
		expectedNameString       string
	}{
		{
			name: "Component found by type",
			collectorConfigStringMap: map[string]any{
				"extensions": map[string]any{
					"exampleextension": map[string]any{},
				},
			},
			typ:                "exampleextension",
			componentsKind:     "extensions",
			expectedConfigured: true,
			expectedTypeString: "exampleextension",
			expectedNameString: "",
		},
		{
			name: "Component found by type and name",
			collectorConfigStringMap: map[string]any{
				"extensions": map[string]any{
					"exampleextension/instance": map[string]any{},
				},
			},
			typ:                "exampleextension",
			componentsKind:     "extensions",
			expectedConfigured: true,
			expectedTypeString: "exampleextension",
			expectedNameString: "instance",
		},
		{
			name: "Component not found",
			collectorConfigStringMap: map[string]any{
				"extensions": map[string]any{
					"otherextension": map[string]any{},
				},
			},
			typ:                "exampleextension",
			componentsKind:     "extensions",
			expectedConfigured: false,
			expectedTypeString: "",
			expectedNameString: "",
		},
		{
			name: "Invalid component map structure",
			collectorConfigStringMap: map[string]any{
				"extensions": []any{
					"exampleextension",
				},
			},
			typ:                "exampleextension",
			componentsKind:     "extensions",
			expectedConfigured: false,
			expectedTypeString: "",
			expectedNameString: "",
		},
		{
			name: "Component kind not found",
			collectorConfigStringMap: map[string]any{
				"receivers": map[string]any{
					"examplereceiver": map[string]any{},
				},
			},
			typ:                "exampleextension",
			componentsKind:     "extensions",
			expectedConfigured: false,
			expectedTypeString: "",
			expectedNameString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured, id := CheckComponentConfiguration(tt.typ, tt.componentsKind, tt.collectorConfigStringMap)
			assert.Equal(t, tt.expectedConfigured, configured)
			if tt.expectedConfigured && configured {
				// Only care about expectedID value if the component is configured
				var expectedID component.ID
				if tt.expectedTypeString != "" {
					if tt.expectedNameString != "" {
						expectedID = component.NewIDWithName(component.MustNewType(tt.expectedTypeString), tt.expectedNameString)
					} else {
						expectedID = component.MustNewID(tt.expectedTypeString)
					}
				} else {
					expectedID = component.ID{}
				}
				assert.Equal(t, &expectedID, id)
			}
		})
	}
}

func TestGetComponentHealthStatus(t *testing.T) {
	tests := []struct {
		name            string
		componentStatus map[string]any
		id              string
		componentsKind  string
		expectedStatus  map[string]any
	}{
		{
			name: "Health status found for extension",
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": map[string]any{
						"components": map[string]any{
							"extension:exampleextension": map[string]any{
								"status": "healthy",
							},
						},
					},
				},
			},
			id:             "exampleextension",
			componentsKind: "extensions",
			expectedStatus: map[string]any{
				"status": "healthy",
			},
		},
		{
			name: "Health status found for receiver in pipeline",
			componentStatus: map[string]any{
				"components": map[string]any{
					"pipeline:traces": map[string]any{
						"components": map[string]any{
							"receiver:examplereceiver": map[string]any{
								"healthy": true,
								"status":  "StatusStarting",
							},
						},
					},
				},
			},
			id:             "examplereceiver",
			componentsKind: "receivers",
			expectedStatus: map[string]any{
				"pipeline:traces": map[string]any{
					"receiver:examplereceiver": map[string]any{
						"healthy": true,
						"status":  "StatusStarting",
					},
				},
			},
		},
		{
			name: "Health status not found",
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": map[string]any{
						"components": map[string]any{
							"extension:otherextension": map[string]any{
								"status": "healthy",
							},
						},
					},
				},
			},
			id:             "exampleextension",
			componentsKind: "extensions",
			expectedStatus: map[string]any{},
		},
		{
			name: "Invalid component kind",
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": map[string]any{
						"components": map[string]any{
							"extension:exampleextension": map[string]any{
								"status": "healthy",
							},
						},
					},
				},
			},
			id:             "exampleextension",
			componentsKind: "invalidkind",
			expectedStatus: nil,
		},
		{
			name: "Invalid component status structure",
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": []any{
						"extension:exampleextension",
					},
				},
			},
			id:             "exampleextension",
			componentsKind: "extensions",
			expectedStatus: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := GetComponentHealthStatus(tt.id, tt.componentsKind, tt.componentStatus)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestGetServiceComponent(t *testing.T) {
	core, observedLogs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	tests := []struct {
		name            string
		componentString string
		componentsKind  string
		moduleInfoJSON  *payload.ModuleInfoJSON
		componentStatus map[string]any
		expectedResult  *payload.ServiceComponent
		expectedLogs    []string
	}{
		{
			name:            "Valid component with no name",
			componentString: "exampleextension",
			componentsKind:  "extensions",
			moduleInfoJSON: func() *payload.ModuleInfoJSON {
				mij := payload.NewModuleInfoJSON()
				mij.AddComponent(payload.CollectorModule{
					Type:    "exampleextension",
					Kind:    "extension",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				return mij
			}(),
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": map[string]any{
						"components": map[string]any{
							"extension:exampleextension": map[string]any{
								"status": "healthy",
							},
						},
					},
				},
			},
			expectedResult: &payload.ServiceComponent{
				ID:              "exampleextension",
				Name:            "",
				Type:            "exampleextension",
				Kind:            "extension",
				Gomod:           "example.com/module",
				Version:         "v1.0.0",
				ComponentStatus: `{"status":"healthy"}`,
			},
			expectedLogs: nil,
		},
		{
			name:            "Valid component with name",
			componentString: "exampleextension/instance",
			componentsKind:  "extensions",
			moduleInfoJSON: func() *payload.ModuleInfoJSON {
				mij := payload.NewModuleInfoJSON()
				mij.AddComponent(payload.CollectorModule{
					Type:    "exampleextension",
					Kind:    "extension",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				return mij
			}(),
			componentStatus: map[string]any{},
			expectedResult: &payload.ServiceComponent{
				ID:              "exampleextension/instance",
				Name:            "instance",
				Type:            "exampleextension",
				Kind:            "extension",
				Gomod:           "example.com/module",
				Version:         "v1.0.0",
				ComponentStatus: "",
			},
			expectedLogs: nil,
		},
		{
			name:            "Invalid component kind",
			componentString: "exampleextension",
			componentsKind:  "invalidkind",
			moduleInfoJSON:  payload.NewModuleInfoJSON(),
			componentStatus: map[string]any{},
			expectedResult:  nil,
			expectedLogs:    []string{"Invalid component kind"},
		},
		{
			name:            "Invalid component ID format",
			componentString: "exampleextension/instance/extra",
			componentsKind:  "extensions",
			moduleInfoJSON:  payload.NewModuleInfoJSON(),
			componentStatus: map[string]any{},
			expectedResult:  nil,
			expectedLogs:    []string{"Invalid component ID"},
		},
		{
			name:            "Component not found in module info",
			componentString: "exampleextension",
			componentsKind:  "extensions",
			moduleInfoJSON:  payload.NewModuleInfoJSON(),
			componentStatus: map[string]any{},
			expectedResult: &payload.ServiceComponent{
				ID:              "exampleextension",
				Name:            "",
				Type:            "exampleextension",
				Kind:            "extension",
				Gomod:           "unknown",
				Version:         "unknown",
				ComponentStatus: "",
			},
			expectedLogs: []string{"service component not found in module info"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observedLogs.TakeAll() // Clear the logs before each test
			result := GetServiceComponent(tt.componentString, tt.componentsKind, tt.moduleInfoJSON, tt.componentStatus, logger)
			assert.Equal(t, tt.expectedResult, result)

			// Verify logs if expected
			if tt.expectedLogs != nil {
				logs := observedLogs.TakeAll()
				assert.Equal(t, len(tt.expectedLogs), len(logs))
				for i, expectedLog := range tt.expectedLogs {
					assert.Contains(t, logs[i].Message, expectedLog)
				}
			}
		})
	}
}

func TestUpdateComponentStatus(t *testing.T) {
	tests := []struct {
		name   string
		source interface {
			Kind() component.Kind
			ComponentID() component.ID
		}
		event interface {
			Status() componentstatus.Status
			Err() error
		}
		expectedStatus map[string]any
	}{
		{
			name: "Update status for receiver",
			source: componentstatus.NewInstanceID(
				component.MustNewID("testreceiver"),
				component.KindReceiver,
			),
			event: componentstatus.NewEvent(componentstatus.StatusOK),
			expectedStatus: map[string]any{
				"receiver:testreceiver": map[string]any{
					"status": componentstatus.StatusOK.String(),
					"error":  nil,
				},
			},
		},
		{
			name: "Update status for processor with error",
			source: componentstatus.NewInstanceID(
				component.MustNewID("testprocessor"),
				component.KindProcessor,
			),
			event: componentstatus.NewEvent(componentstatus.StatusRecoverableError),
			expectedStatus: map[string]any{
				"processor:testprocessor": map[string]any{
					"status": componentstatus.StatusRecoverableError.String(),
					"error":  nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UpdateComponentStatus(tt.source, tt.event)

			// Check that the expected status fields match
			for key, expectedValue := range tt.expectedStatus {
				actualValue, exists := result[key]
				assert.True(t, exists, "Expected key %s not found in result", key)
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
		})
	}
}

func TestDataToFlattenedJSONString(t *testing.T) {
	tests := []struct {
		name        string
		data        any
		removeLines bool
		expected    string
	}{
		{
			name: "Simple map with lines and quotes",
			data: map[string]any{
				"key": "value",
			},
			removeLines: false,
			expected: `{
  "key": "value"
}`,
		},
		{
			name: "Simple map without lines",
			data: map[string]any{
				"key": "value",
			},
			removeLines: true,
			expected:    `{"key":"value"}`,
		},
		{
			name:        "Invalid JSON",
			data:        make(chan int),
			removeLines: false,
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DataToFlattenedJSONString(tt.data, tt.removeLines)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPopulateFullComponentsJSON(t *testing.T) {
	tests := []struct {
		name                     string
		moduleInfo               service.ModuleInfos
		collectorConfigStringMap map[string]any
		components               []payload.CollectorModule
	}{
		{
			name: "All component types included",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("examplereceiver"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Processor: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleprocessor"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Exporter: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleexporter"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Extension: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleextension"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Connector: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleconnector"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			collectorConfigStringMap: map[string]any{
				"receivers": map[string]any{
					"examplereceiver": map[string]any{},
				},
				"processors": map[string]any{
					"exampleprocessor": map[string]any{},
				},
				"exporters": map[string]any{
					"exampleexporter": map[string]any{},
				},
				"extensions": map[string]any{
					"exampleextension": map[string]any{},
				},
				"connectors": map[string]any{
					"exampleconnector": map[string]any{},
				},
			},
			components: []payload.CollectorModule{
				{
					Type:       "examplereceiver",
					Kind:       "receiver",
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleprocessor",
					Kind:       "processor",
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleexporter",
					Kind:       "exporter",
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleextension",
					Kind:       "extension",
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleconnector",
					Kind:       "connector",
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
			},
		},
		{
			name: "No components included",
			moduleInfo: service.ModuleInfos{
				Receiver:  map[component.Type]service.ModuleInfo{},
				Processor: map[component.Type]service.ModuleInfo{},
				Exporter:  map[component.Type]service.ModuleInfo{},
				Extension: map[component.Type]service.ModuleInfo{},
				Connector: map[component.Type]service.ModuleInfo{},
			},
			collectorConfigStringMap: map[string]any{},
			components:               []payload.CollectorModule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modInfo := PopulateFullComponentsJSON(tt.moduleInfo, tt.collectorConfigStringMap)
			assert.ElementsMatch(t, tt.components, modInfo.GetFullComponentsList())
		})
	}
}

func TestPopulateActiveComponentsJSON(t *testing.T) {
	tests := []struct {
		name                     string
		collectorConfigStringMap map[string]any
		moduleInfoJSON           *payload.ModuleInfoJSON
		componentStatus          map[string]any
		expectedComponents       []payload.ServiceComponent
		expectedError            error
	}{
		{
			name: "All component types included",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": []any{
						"exampleextension",
					},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{
								"examplereceiver",
							},
							"processors": []any{
								"exampleprocessor",
							},
							"exporters": []any{
								"exampleexporter",
							},
						},
					},
				},
			},
			moduleInfoJSON: func() *payload.ModuleInfoJSON {
				mij := payload.NewModuleInfoJSON()
				mij.AddComponent(payload.CollectorModule{
					Type:    "exampleextension",
					Kind:    "extension",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				mij.AddComponent(payload.CollectorModule{
					Type:    "examplereceiver",
					Kind:    "receiver",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				mij.AddComponent(payload.CollectorModule{
					Type:    "exampleprocessor",
					Kind:    "processor",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				mij.AddComponent(payload.CollectorModule{
					Type:    "exampleexporter",
					Kind:    "exporter",
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				})
				return mij
			}(),
			componentStatus: map[string]any{
				"components": map[string]any{
					"extensions": map[string]any{
						"components": map[string]any{
							"extension:exampleextension": map[string]any{
								"status": "healthy",
							},
						},
					},
					"pipeline:traces": map[string]any{
						"components": map[string]any{
							"receiver:examplereceiver": map[string]any{
								"healthy": true,
								"status":  "StatusStarting",
							},
							"processor:exampleprocessor": map[string]any{
								"healthy": true,
								"status":  "StatusStarting",
							},
							"exporter:exampleexporter": map[string]any{
								"healthy": true,
								"status":  "StatusStarting",
							},
						},
					},
				},
			},
			expectedComponents: []payload.ServiceComponent{
				{
					ID:              "exampleextension",
					Name:            "",
					Type:            "exampleextension",
					Kind:            "extension",
					Gomod:           "example.com/module",
					Version:         "v1.0.0",
					ComponentStatus: `{"status":"healthy"}`,
				},
				{
					ID:              "examplereceiver",
					Name:            "",
					Type:            "examplereceiver",
					Kind:            "receiver",
					Gomod:           "example.com/module",
					Version:         "v1.0.0",
					ComponentStatus: `{"pipeline:traces":{"receiver:examplereceiver":{"healthy":true,"status":"StatusStarting"}}}`,
				},
				{
					ID:              "exampleprocessor",
					Name:            "",
					Type:            "exampleprocessor",
					Kind:            "processor",
					Gomod:           "example.com/module",
					Version:         "v1.0.0",
					ComponentStatus: `{"pipeline:traces":{"processor:exampleprocessor":{"healthy":true,"status":"StatusStarting"}}}`,
				},
				{
					ID:              "exampleexporter",
					Name:            "",
					Type:            "exampleexporter",
					Kind:            "exporter",
					Gomod:           "example.com/module",
					Version:         "v1.0.0",
					ComponentStatus: `{"pipeline:traces":{"exporter:exampleexporter":{"healthy":true,"status":"StatusStarting"}}}`,
				},
			},
			expectedError: nil,
		},
		{
			name: "No service map",
			collectorConfigStringMap: map[string]any{
				"receivers": map[string]any{},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      errors.New("failed to get service map from collector config, cannot populate active components table"),
		},
		{
			name: "Invalid extensions list",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": map[string]any{},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
		{
			name: "Invalid extension value",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": []any{
						123,
					},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
		{
			name: "Invalid pipeline map",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": []any{},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
		{
			name: "Invalid pipeline components map",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": []any{},
					},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
		{
			name: "Invalid pipeline components list",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": map[string]any{},
						},
					},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
		{
			name: "Invalid pipeline component value",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{
								123,
							},
						},
					},
				},
			},
			moduleInfoJSON:     payload.NewModuleInfoJSON(),
			componentStatus:    map[string]any{},
			expectedComponents: []payload.ServiceComponent{},
			expectedError:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activeComponents, err := PopulateActiveComponentsJSON(tt.collectorConfigStringMap, tt.moduleInfoJSON, tt.componentStatus, zap.NewNop())
			if tt.expectedError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedComponents, activeComponents.Components)
			}
		})
	}
}
