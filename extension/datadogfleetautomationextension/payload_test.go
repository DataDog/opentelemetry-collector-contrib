// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewModuleInfoJSON(t *testing.T) {
	modInfo := newModuleInfoJSON()
	assert.NotNil(t, modInfo)
	assert.NotNil(t, modInfo.components)
	assert.Equal(t, 0, len(modInfo.components))
}

func TestAddComponent(t *testing.T) {
	modInfo := newModuleInfoJSON()
	comp := collectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.addComponent(comp)
	assert.Equal(t, 1, len(modInfo.components))
	key := modInfo.getKey(comp.Type, comp.Kind)
	assert.Equal(t, comp, modInfo.components[key])
}

func TestGetComponent(t *testing.T) {
	modInfo := newModuleInfoJSON()
	comp := collectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.addComponent(comp)
	retrievedComp, ok := modInfo.getComponent("receiver", "otlp")
	assert.True(t, ok)
	assert.Equal(t, comp, retrievedComp)

	_, ok = modInfo.getComponent("processor", "batch")
	assert.False(t, ok)
}

func TestMarshalJSON(t *testing.T) {
	modInfo := newModuleInfoJSON()
	comp1 := collectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	comp2 := collectorModule{
		Type:       "processor",
		Kind:       "batch",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/processor/batchprocessor",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.addComponent(comp1)
	modInfo.addComponent(comp2)

	jsonData, err := json.Marshal(modInfo)
	assert.NoError(t, err)

	var actualJSON map[string]any
	err = json.Unmarshal(jsonData, &actualJSON)
	assert.NoError(t, err)

	expectedJSON := `{
			"full_components": [
					{
							"type": "receiver",
							"kind": "otlp",
							"gomod": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
							"version": "v0.30.0",
							"configured": true
					},
					{
							"type": "processor",
							"kind": "batch",
							"gomod": "github.com/open-telemetry/opentelemetry-collector-contrib/processor/batchprocessor",
							"version": "v0.30.0",
							"configured": true
					}
			]
	}`
	var expectedJSONMap map[string]any
	err = json.Unmarshal([]byte(expectedJSON), &expectedJSONMap)
	assert.NoError(t, err)

	assert.ElementsMatch(t,expectedJSONMap["full_components"], actualJSON["full_components"])
}
