// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"errors"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestIsComponentConfigured(t *testing.T) {
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
			e := &fleetAutomationExtension{
				collectorConfigStringMap: tt.collectorConfigStringMap,
			}
			configured, id := e.isComponentConfigured(tt.typ, tt.componentsKind)
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

func TestIsModuleAvailable(t *testing.T) {
	tests := []struct {
		name              string
		moduleInfo        service.ModuleInfos
		componentType     string
		componentKind     string
		expectedAvailable bool
	}{
		{
			name: "Module available for receiver",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("examplereceiver"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "examplereceiver",
			componentKind:     receiverKind,
			expectedAvailable: true,
		},
		{
			name: "Module not available for receiver",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{},
			},
			componentType:     "examplereceiver",
			componentKind:     receiverKind,
			expectedAvailable: false,
		},
		{
			name: "Module available for processor",
			moduleInfo: service.ModuleInfos{
				Processor: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleprocessor"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "exampleprocessor",
			componentKind:     processorKind,
			expectedAvailable: true,
		},
		{
			name: "Module not available for processor",
			moduleInfo: service.ModuleInfos{
				Processor: map[component.Type]service.ModuleInfo{},
			},
			componentType:     "exampleprocessor",
			componentKind:     processorKind,
			expectedAvailable: false,
		},
		{
			name: "Module available for exporter",
			moduleInfo: service.ModuleInfos{
				Exporter: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleexporter"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "exampleexporter",
			componentKind:     exporterKind,
			expectedAvailable: true,
		},
		{
			name: "Module not available for exporter",
			moduleInfo: service.ModuleInfos{
				Exporter: map[component.Type]service.ModuleInfo{},
			},
			componentType:     "exampleexporter",
			componentKind:     exporterKind,
			expectedAvailable: false,
		},
		{
			name: "Module available for extension",
			moduleInfo: service.ModuleInfos{
				Extension: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleextension"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "exampleextension",
			componentKind:     extensionKind,
			expectedAvailable: true,
		},
		{
			name: "Module not available for extension",
			moduleInfo: service.ModuleInfos{
				Extension: map[component.Type]service.ModuleInfo{},
			},
			componentType:     "exampleextension",
			componentKind:     extensionKind,
			expectedAvailable: false,
		},
		{
			name: "Module available for connector",
			moduleInfo: service.ModuleInfos{
				Connector: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleconnector"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "exampleconnector",
			componentKind:     connectorKind,
			expectedAvailable: true,
		},
		{
			name: "Module not available for connector",
			moduleInfo: service.ModuleInfos{
				Connector: map[component.Type]service.ModuleInfo{},
			},
			componentType:     "exampleconnector",
			componentKind:     connectorKind,
			expectedAvailable: false,
		},
		{
			name: "Component kind not found",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("examplereceiver"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "exampleextension",
			componentKind:     extensionKind,
			expectedAvailable: false,
		},
		{
			name: "otlp receiver available, but looking for otlp exporter",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("otlp"): {BuilderRef: "example.com/module v1.0.0"},
				},
			},
			componentType:     "otlp",
			componentKind:     exporterKind,
			expectedAvailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fleetAutomationExtension{
				moduleInfo: tt.moduleInfo,
			}
			available := e.isModuleAvailable(tt.componentType, tt.componentKind)
			assert.Equal(t, tt.expectedAvailable, available)
		})
	}
}

func TestIsHealthCheckV2Enabled(t *testing.T) {
	tests := []struct {
		name                 string
		healthCheckV2Config  map[string]any
		expectedEnabled      bool
		expectedErrorMessage string
	}{
		{
			name: "HealthCheckV2 enabled with HTTP status check enabled",
			healthCheckV2Config: map[string]any{
				"use_v2": true,
				"http": map[string]any{
					"status": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedEnabled:      true,
			expectedErrorMessage: "",
		},
		{
			name: "HealthCheckV2 enabled but HTTP status check not enabled",
			healthCheckV2Config: map[string]any{
				"use_v2": true,
				"http": map[string]any{
					"status": map[string]any{
						"enabled": false,
					},
				},
			},
			expectedEnabled:      false,
			expectedErrorMessage: "healthcheckv2 extension is enabled but http status check is not enabled; component status will not be available",
		},
		{
			name: "HealthCheckV2 enabled but HTTP status not configured",
			healthCheckV2Config: map[string]any{
				"use_v2": true,
				"http":   map[string]any{},
			},
			expectedEnabled:      false,
			expectedErrorMessage: "healthcheckv2 extension is enabled but http status is not configured; component status will not be available",
		},
		{
			name: "HealthCheckV2 enabled but HTTP endpoint not configured",
			healthCheckV2Config: map[string]any{
				"use_v2": true,
			},
			expectedEnabled:      false,
			expectedErrorMessage: "healthcheckv2 extension is enabled but http endpoint is not configured; component status will not be available",
		},
		{
			name: "HealthCheckV2 enabled but set to legacy mode",
			healthCheckV2Config: map[string]any{
				"use_v2": false,
			},
			expectedEnabled:      false,
			expectedErrorMessage: "healthcheckv2 extension is enabled but is set to legacy mode; component status will not be available",
		},
		{
			name:                 "HealthCheckV2 not enabled",
			healthCheckV2Config:  map[string]any{},
			expectedEnabled:      false,
			expectedErrorMessage: "healthcheckv2 extension is enabled but is set to legacy mode; component status will not be available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fleetAutomationExtension{
				healthCheckV2Config: tt.healthCheckV2Config,
			}
			enabled, err := e.isHealthCheckV2Enabled()
			assert.Equal(t, tt.expectedEnabled, enabled)
			if tt.expectedErrorMessage != "" {
				assert.EqualError(t, err, tt.expectedErrorMessage)
			} else {
				assert.NoError(t, err)
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
			e := &fleetAutomationExtension{
				componentStatus: tt.componentStatus,
			}
			status := e.getComponentHealthStatus(tt.id, tt.componentsKind)
			assert.Equal(t, tt.expectedStatus, status)
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
				"extensions": map[string]any{
					"exampleextension": map[string]any{},
				},
				"receivers": map[string]any{
					"examplereceiver": map[string]any{},
				},
				"processors": map[string]any{
					"exampleprocessor": map[string]any{},
				},
			},
			components: []payload.CollectorModule{
				{
					Type:       "examplereceiver",
					Kind:       receiverKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleprocessor",
					Kind:       processorKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleexporter",
					Kind:       exporterKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: false,
				},
				{
					Type:       "exampleextension",
					Kind:       extensionKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleconnector",
					Kind:       connectorKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: false,
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
		{
			name: "Some components included",
			moduleInfo: service.ModuleInfos{
				Receiver: map[component.Type]service.ModuleInfo{
					component.MustNewType("examplereceiver"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Processor: map[component.Type]service.ModuleInfo{},
				Exporter: map[component.Type]service.ModuleInfo{
					component.MustNewType("exampleexporter"): {BuilderRef: "example.com/module v1.0.0"},
				},
				Extension: map[component.Type]service.ModuleInfo{},
				Connector: map[component.Type]service.ModuleInfo{},
			},
			collectorConfigStringMap: map[string]any{
				"receivers": map[string]any{
					"examplereceiver": map[string]any{},
				},
				"exporters": map[string]any{
					"exampleexporter": map[string]any{},
				},
			},
			components: []payload.CollectorModule{
				{
					Type:       "examplereceiver",
					Kind:       receiverKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleexporter",
					Kind:       exporterKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fleetAutomationExtension{
				moduleInfo:               tt.moduleInfo,
				collectorConfigStringMap: tt.collectorConfigStringMap,
			}
			output := e.populateFullComponentsJSON()
			expectedOutput := payload.NewModuleInfoJSON()
			expectedOutput.PutComponents(tt.components)
			assert.Equal(t, expectedOutput, output)
		})
	}
}

func TestGetServiceComponent(t *testing.T) {
	tests := []struct {
		name                     string
		componentStatus          map[string]any
		healthCheckV2Enabled     bool
		components               []payload.CollectorModule
		componentString          string
		componentsKind           string
		expectedServiceComponent *payload.ServiceComponent
	}{
		{
			name: "Service component found without name",
			components: []payload.CollectorModule{
				{
					Type:    "examplereceiver",
					Kind:    receiverKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
			},
			componentStatus:      map[string]any{},
			healthCheckV2Enabled: false,
			componentString:      "examplereceiver",
			componentsKind:       receiversKind,
			expectedServiceComponent: &payload.ServiceComponent{
				ID:      "examplereceiver",
				Name:    "",
				Type:    "examplereceiver",
				Kind:    receiverKind,
				Gomod:   "example.com/module",
				Version: "v1.0.0",
			},
		},
		{
			name: "Service component found with name",
			components: []payload.CollectorModule{
				{
					Type:    "exampleprocessor",
					Kind:    processorKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
			},
			componentStatus:      map[string]any{},
			healthCheckV2Enabled: false,
			componentString:      "exampleprocessor/instance",
			componentsKind:       processorsKind,
			expectedServiceComponent: &payload.ServiceComponent{
				ID:      "exampleprocessor/instance",
				Name:    "instance",
				Type:    "exampleprocessor",
				Kind:    processorKind,
				Gomod:   "example.com/module",
				Version: "v1.0.0",
			},
		},
		{
			name:                 "Service component not found",
			components:           []payload.CollectorModule{},
			componentStatus:      map[string]any{},
			healthCheckV2Enabled: false,
			componentString:      "exampleextension",
			componentsKind:       extensionsKind,
			expectedServiceComponent: &payload.ServiceComponent{
				ID:      "exampleextension",
				Name:    "",
				Type:    "exampleextension",
				Kind:    extensionKind,
				Gomod:   "unknown",
				Version: "unknown",
			},
		},
		{
			name:                     "Invalid component kind",
			components:               []payload.CollectorModule{},
			componentStatus:          map[string]any{},
			healthCheckV2Enabled:     false,
			componentString:          "exampleextension",
			componentsKind:           "invalidkind",
			expectedServiceComponent: nil,
		},
		{
			name: "Health check status enabled and found",
			components: []payload.CollectorModule{
				{
					Type:    "exampleextension",
					Kind:    extensionKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
			},
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
			healthCheckV2Enabled: true,
			componentString:      "exampleextension",
			componentsKind:       extensionsKind,
			expectedServiceComponent: &payload.ServiceComponent{
				ID:              "exampleextension",
				Name:            "",
				Type:            "exampleextension",
				Kind:            extensionKind,
				Gomod:           "example.com/module",
				Version:         "v1.0.0",
				ComponentStatus: "{\"status\": \"healthy\"}",
			},
		},
		{
			name: "Health check status enabled but not found",
			components: []payload.CollectorModule{
				{
					Type:    "exampleextension",
					Kind:    extensionKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
			},
			componentStatus:      map[string]any{},
			healthCheckV2Enabled: true,
			componentString:      "exampleextension",
			componentsKind:       extensionsKind,
			expectedServiceComponent: &payload.ServiceComponent{
				ID:              "exampleextension",
				Name:            "",
				Type:            "exampleextension",
				Kind:            extensionKind,
				Gomod:           "example.com/module",
				Version:         "v1.0.0",
				ComponentStatus: "",
			},
		},
		{
			name:                     "Invalid component ID",
			componentString:          "extension/1/2",
			componentsKind:           extensionsKind,
			expectedServiceComponent: nil,
		},
	}
	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moduleInfoJSON := payload.NewModuleInfoJSON()
			moduleInfoJSON.PutComponents(tt.components)
			e := &fleetAutomationExtension{
				ModuleInfoJSON:       moduleInfoJSON,
				componentStatus:      tt.componentStatus,
				healthCheckV2Enabled: tt.healthCheckV2Enabled,
				telemetry: component.TelemetrySettings{
					Logger: logger,
				},
			}
			serviceComponent := e.getServiceComponent(tt.componentString, tt.componentsKind)
			assert.Equal(t, tt.expectedServiceComponent, serviceComponent)
		})
	}
}

func TestDataToFlattenedJSONString(t *testing.T) {
	tests := []struct {
		name           string
		data           any
		removeNewLines bool
		removeQuotes   bool
		expectedOutput string
	}{
		{
			name: "Simple map without removing newlines and quotes",
			data: map[string]any{
				"key": "value",
			},
			removeNewLines: false,
			removeQuotes:   false,
			expectedOutput: "{\n  \"key\": \"value\"\n}",
		},
		{
			name: "Simple map with removing newlines and quotes",
			data: map[string]any{
				"key": "value",
			},
			removeNewLines: true,
			removeQuotes:   true,
			expectedOutput: "{key: value}",
		},
		{
			name: "Nested map without removing newlines and quotes",
			data: map[string]any{
				"key": map[string]any{
					"nestedKey": "nestedValue",
				},
			},
			removeNewLines: false,
			removeQuotes:   false,
			expectedOutput: "{\n  \"key\": {\n    \"nestedKey\": \"nestedValue\"\n  }\n}",
		},
		{
			name: "Nested map with removing newlines and quotes",
			data: map[string]any{
				"key": map[string]any{
					"nestedKey": "nestedValue",
				},
			},
			removeNewLines: true,
			removeQuotes:   true,
			expectedOutput: "{key: {nestedKey: nestedValue}}",
		},
		{
			name:           "Empty map",
			data:           map[string]any{},
			removeNewLines: false,
			removeQuotes:   false,
			expectedOutput: "{}",
		},
		{
			name:           "Nil data",
			data:           nil,
			removeNewLines: false,
			removeQuotes:   false,
			expectedOutput: "null",
		},
		{
			name: "removeNewLines && removeQuotes, json.Marshal fails",
			data: map[string]any{
				"key": make(chan int),
			},
			removeNewLines: true,
			removeQuotes:   true,
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := dataToFlattenedJSONString(tt.data, tt.removeNewLines, tt.removeQuotes)
			assert.Equal(t, tt.expectedOutput, output)
		})
	}
}

func TestPopulateActiveComponentsJSON(t *testing.T) {
	tests := []struct {
		name                     string
		collectorConfigStringMap map[string]any
		components               []payload.CollectorModule
		expectedComponents       []payload.ServiceComponent
		expectedError            error
		expectedLogs             []string
	}{
		{
			name: "Valid service map with extensions and pipelines",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": []any{"exampleextension"},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"examplereceiver"},
							"processors": []any{"exampleprocessor"},
							"exporters":  []any{"exampleexporter"},
						},
					},
				},
			},
			components: []payload.CollectorModule{
				{
					Type:       "exampleextension",
					Kind:       extensionKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "examplereceiver",
					Kind:       receiverKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleprocessor",
					Kind:       processorKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
				{
					Type:       "exampleexporter",
					Kind:       exporterKind,
					Gomod:      "example.com/module",
					Version:    "v1.0.0",
					Configured: true,
				},
			},
			expectedComponents: []payload.ServiceComponent{
				{
					ID:      "exampleextension",
					Name:    "",
					Type:    "exampleextension",
					Kind:    extensionKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
				{
					ID:      "examplereceiver",
					Name:    "",
					Type:    "examplereceiver",
					Kind:    receiverKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
				{
					ID:      "exampleprocessor",
					Name:    "",
					Type:    "exampleprocessor",
					Kind:    processorKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
				{
					ID:      "exampleexporter",
					Name:    "",
					Type:    "exampleexporter",
					Kind:    exporterKind,
					Gomod:   "example.com/module",
					Version: "v1.0.0",
				},
			},
			expectedError: nil,
			expectedLogs:  []string{},
		},
		{
			name: "Invalid service map structure",
			collectorConfigStringMap: map[string]any{
				"service": "invalid",
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      errors.New("failed to get service map from collector config, cannot populate active components table"),
			expectedLogs:       []string{"Failed to get service map from collector config, cannot populate active components table"},
		},
		{
			name: "Invalid extensions list structure",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": "invalid",
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Failed to get extensions list from service map"},
		},
		{
			name: "Invalid extension value type",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"extensions": []any{123},
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Extensions list in service map config contains non-string value"},
		},
		{
			name: "Invalid pipelines map structure",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": "invalid",
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Failed to get pipeline map from service map config"},
		},
		{
			name: "Invalid components map structure in pipeline",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": "invalid",
					},
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Failed to get components map from pipeline map in service map config"},
		},
		{
			name: "Invalid component value type in pipeline",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{123},
						},
					},
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Components list in pipeline map in service map config contains non-string value"},
		},
		{
			name: "pipelinesKind componentsInterface does not cast to []any",
			collectorConfigStringMap: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": "invalid",
						},
					},
				},
			},
			components:         []payload.CollectorModule{},
			expectedComponents: []payload.ServiceComponent(nil),
			expectedError:      nil,
			expectedLogs:       []string{"Failed to get components list from pipeline map in service map config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)
			moduleInfoJSON := payload.NewModuleInfoJSON()
			moduleInfoJSON.PutComponents(tt.components)
			e := &fleetAutomationExtension{
				collectorConfigStringMap: tt.collectorConfigStringMap,
				ModuleInfoJSON:           moduleInfoJSON,
				telemetry: component.TelemetrySettings{
					Logger: logger,
				},
			}

			components, err := e.populateActiveComponentsJSON()
			assert.ElementsMatch(t, tt.expectedComponents, components.Components)
			assert.Equal(t, tt.expectedError, err)

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
