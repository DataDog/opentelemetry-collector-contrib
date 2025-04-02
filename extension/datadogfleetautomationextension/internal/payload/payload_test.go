// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package payload

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewModuleInfoJSON(t *testing.T) {
	modInfo := NewModuleInfoJSON()
	assert.NotNil(t, modInfo)
	assert.NotNil(t, modInfo.components)
	assert.Empty(t, modInfo.components)
}

func TestAddComponent(t *testing.T) {
	modInfo := NewModuleInfoJSON()
	comp := CollectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.AddComponent(comp)
	assert.Len(t, modInfo.components, 1)
	key := modInfo.getKey(comp.Type, comp.Kind)
	assert.Equal(t, comp, modInfo.components[key])
}

func TestGetComponent(t *testing.T) {
	modInfo := NewModuleInfoJSON()
	comp := CollectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.AddComponent(comp)
	retrievedComp, ok := modInfo.GetComponent("receiver", "otlp")
	assert.True(t, ok)
	assert.Equal(t, comp, retrievedComp)

	_, ok = modInfo.GetComponent("processor", "batch")
	assert.False(t, ok)
}

func TestMarshalJSON(t *testing.T) {
	modInfo := NewModuleInfoJSON()
	comp1 := CollectorModule{
		Type:       "receiver",
		Kind:       "otlp",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver",
		Version:    "v0.30.0",
		Configured: true,
	}
	comp2 := CollectorModule{
		Type:       "processor",
		Kind:       "batch",
		Gomod:      "github.com/open-telemetry/opentelemetry-collector-contrib/processor/batchprocessor",
		Version:    "v0.30.0",
		Configured: true,
	}
	modInfo.AddComponent(comp1)
	modInfo.AddComponent(comp2)

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
	ap := &AgentPayload{}
	_, err := ap.SplitPayload(1)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not split inventories agent payload any more, payload is too big for intake")
	op := &OtelAgentPayload{}
	_, err = op.SplitPayload(1)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not split inventories otel payload any more, payload is too big for intake")
}

func TestPrepareAgentMetadataPayload(t *testing.T) {
	site := "datadoghq.com"
	tool := "otelcol"
	toolVersion := "1.0.0"
	installerVersion := "1.0.0"
	hostname := "test-hostname"

	expectedPayload := AgentMetadata{
		AgentVersion:                      "7.64.0-collector",
		AgentStartupTimeMs:                1234567890123,
		AgentFlavor:                       "",
		ConfigSite:                        site,
		ConfigEKSFargate:                  false,
		InstallMethodTool:                 tool,
		InstallMethodToolVersion:          toolVersion,
		InstallMethodInstallerVersion:     installerVersion,
		FeatureRemoteConfigurationEnabled: true,
		FeatureOTLPEnabled:                true,
		Hostname:                          hostname,
	}

	actualPayload := PrepareAgentMetadataPayload(site, tool, toolVersion, installerVersion, hostname)

	assert.Equal(t, expectedPayload, actualPayload)
}

func TestPrepareOtelMetadataPayload(t *testing.T) {
	version := "1.0.0"
	extensionVersion := "1.0.0"
	command := "otelcol"
	fullConfig := "{\"service\":{\"pipelines\":{\"traces\":{\"receivers\":[\"otlp\"],\"exporters\":[\"debug\"]}}}}"

	expectedPayload := OtelMetadata{
		Enabled:                          true,
		Version:                          version,
		ExtensionVersion:                 extensionVersion,
		Command:                          command,
		Description:                      "OSS Collector with Datadog Fleet Automation Extension",
		ProvidedConfiguration:            "",
		EnvironmentVariableConfiguration: "",
		FullConfiguration:                fullConfig,
	}

	actualPayload := PrepareOtelMetadataPayload(version, extensionVersion, command, fullConfig)

	assert.Equal(t, expectedPayload, actualPayload)
}

func TestPrepareOtelCollectorPayload(t *testing.T) {
	hostname := "test-hostname"
	hostnameSource := "config"
	extensionUUID := "test-uuid"
	version := "1.0.0"
	site := "datadoghq.com"
	fullConfig := "{\"service\":{\"pipelines\":{\"traces\":{\"receivers\":[\"otlp\"],\"exporters\":[\"debug\"]}}}}"
	buildInfo := CustomBuildInfo{
		Command: "otelcol",
		Version: "1.0.0",
	}

	expectedPayload := OtelCollector{
		HostKey:           "",
		Hostname:          hostname,
		HostnameSource:    hostnameSource,
		CollectorID:       hostname + "-" + extensionUUID,
		CollectorVersion:  version,
		ConfigSite:        site,
		APIKeyUUID:        "",
		BuildInfo:         buildInfo,
		FullConfiguration: fullConfig,
	}

	actualPayload := PrepareOtelCollectorPayload(hostname, hostnameSource, extensionUUID, version, site, fullConfig, buildInfo)

	assert.Equal(t, expectedPayload, actualPayload)
}

func TestModuleInfoJSON_PutComponents(t *testing.T) {
	mi := NewModuleInfoJSON()
	components := []CollectorModule{
		{
			Type:       "test_type",
			Kind:       "test_kind",
			Gomod:      "test_gomod",
			Version:    "test_version",
			Configured: true,
		},
	}
	mi.PutComponents(components)
	assert.Equal(t, components[0], mi.components["test_type:test_kind"])
}

func TestModuleInfoJSON_GetFullComponentsList(t *testing.T) {
	mi := NewModuleInfoJSON()
	components := []CollectorModule{
		{
			Type:       "test_type",
			Kind:       "test_kind",
			Gomod:      "test_gomod",
			Version:    "test_version",
			Configured: true,
		},
	}
	mi.PutComponents(components)
	fullList := mi.GetFullComponentsList()
	assert.Equal(t, components, fullList)
}

func TestModuleInfoJSON_MarshalJSON(t *testing.T) {
	mi := NewModuleInfoJSON()
	components := []CollectorModule{
		{
			Type:       "test_type",
			Kind:       "test_kind",
			Gomod:      "test_gomod",
			Version:    "test_version",
			Configured: true,
		},
	}
	mi.PutComponents(components)

	jsonData, err := json.Marshal(mi)
	require.NoError(t, err)

	var unmarshaled struct {
		Components []CollectorModule `json:"full_components"`
	}
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, components, unmarshaled.Components)
}

func TestActiveComponentsJSON_MarshalJSON(t *testing.T) {
	ac := &ActiveComponentsJSON{
		Components: []ServiceComponent{
			{
				ID:              "test_id",
				Name:            "test_name",
				Type:            "test_type",
				Kind:            "test_kind",
				Pipeline:        "test_pipeline",
				Gomod:           "test_gomod",
				Version:         "test_version",
				ComponentStatus: "test_status",
			},
		},
	}

	jsonData, err := json.Marshal(ac)
	require.NoError(t, err)

	var unmarshaled ActiveComponentsJSON
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, ac.Components, unmarshaled.Components)
}

func TestOtelCollectorPayload_MarshalJSON(t *testing.T) {
	oc := &OtelCollectorPayload{
		Hostname:  "test_host",
		Timestamp: time.Now().UnixNano(),
		UUID:      "test-uuid",
		Metadata: OtelCollector{
			FullComponents: []CollectorModule{
				{
					Type:       "test_type",
					Kind:       "test_kind",
					Gomod:      "test_gomod",
					Version:    "test_version",
					Configured: true,
				},
			},
		},
	}

	jsonData, err := json.Marshal(oc)
	require.NoError(t, err)

	var unmarshaled OtelCollectorPayload
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, oc.Hostname, unmarshaled.Hostname)
	assert.Equal(t, oc.UUID, unmarshaled.UUID)
	assert.Equal(t, oc.Metadata.FullComponents, unmarshaled.Metadata.FullComponents)
}
