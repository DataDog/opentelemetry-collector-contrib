// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewModuleInfoJSON(t *testing.T) {
	modInfo := newModuleInfoJSON()
	assert.NotNil(t, modInfo)
	assert.NotNil(t, modInfo.components)
	assert.Empty(t, modInfo.components)
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
	assert.Len(t, modInfo.components, 1)
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

	assert.ElementsMatch(t, expectedJSONMap["full_components"], actualJSON["full_components"])
}

func TestSplitPayloadInterfaces(t *testing.T) {
	ap := &agentPayload{}
	_, err := ap.SplitPayload(1)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not split inventories agent payload any more, payload is too big for intake")
	op := &otelAgentPayload{}
	_, err = op.SplitPayload(1)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not split inventories otel payload any more, payload is too big for intake")
}
