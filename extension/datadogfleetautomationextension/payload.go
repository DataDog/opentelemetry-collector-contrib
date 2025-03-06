// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"go.opentelemetry.io/collector/component"
)

type OtelCollector struct {
	HostKey           string              `json:"host_key"`
	Hostname          string              `json:"hostname"`
	HostnameSource    string              `json:"hostname_source"`
	CollectorID       string              `json:"collector_id"`
	CollectorVersion  string              `json:"collector_version"`
	ConfigSite        string              `json:"config_site"`
	APIKeyUUID        string              `json:"api_key_uuid"`
	FullComponents    []collectorModule   `json:"full_components"`
	ActiveComponents  []serviceComponent  `json:"active_components"`
	BuildInfo         component.BuildInfo `json:"build_info"`
	FullConfiguration string              `json:"full_configuration"` // JSON passed as string
	HealthStatus      string              `json:"health_status"`      // JSON passed as string
}

type OtelMetadata struct {
	Command                          string `json:"command"`
	Description                      string `json:"description"`
	Enabled                          bool   `json:"enabled"`
	EnvironmentVariableConfiguration string `json:"environment_variable_configuration"`
	ExtensionVersion                 string `json:"extension_version"`
	FullConfiguration                string `json:"full_configuration"`
	ProvidedConfiguration            string `json:"provided_configuration"`
	RuntimeOverrideConfiguration     string `json:"runtime_override_configuration"`
	Version                          string `json:"version"`
}

type CombinedPayload struct {
	OtelPayload  otelAgentPayload `json:"otel_payload"`
	AgentPayload agentPayload     `json:"agent_payload"`
}

type AgentMetadata struct {
	AgentVersion                           string   `json:"agent_version,omitempty"`
	AgentStartupTimeMs                     int64    `json:"agent_startup_time_ms,omitempty"`
	AgentFlavor                            string   `json:"flavor,omitempty"`
	ConfigAPMDDUrl                         string   `json:"config_apm_dd_url,omitempty"`
	ConfigDDUrl                            string   `json:"config_dd_url,omitempty"`
	ConfigSite                             string   `json:"config_site,omitempty"`
	ConfigLogsDDUrl                        string   `json:"config_logs_dd_url,omitempty"`
	ConfigLogsSocks5ProxyAddress           string   `json:"config_logs_socks5_proxy_address,omitempty"`
	ConfigNoProxy                          []string `json:"config_no_proxy,omitempty"`
	ConfigProcessDDUrl                     string   `json:"config_process_dd_url,omitempty"`
	ConfigProxyHTTP                        string   `json:"config_proxy_http,omitempty"`
	ConfigProxyHTTPS                       string   `json:"config_proxy_https,omitempty"`
	ConfigEKSFargate                       bool     `json:"config_eks_fargate,omitempty"`
	InstallMethodTool                      string   `json:"install_method_tool,omitempty"`
	InstallMethodToolVersion               string   `json:"install_method_tool_version,omitempty"`
	InstallMethodInstallerVersion          string   `json:"install_method_installer_version,omitempty"`
	LogsTransport                          string   `json:"logs_transport,omitempty"`
	FeatureFIPSEnabled                     bool     `json:"feature_fips_enabled,omitempty"`
	FeatureCWSEnabled                      bool     `json:"feature_cws_enabled,omitempty"`
	FeatureCWSNetworkEnabled               bool     `json:"feature_cws_network_enabled,omitempty"`
	FeatureCWSSecurityProfilesEnabled      bool     `json:"feature_cws_security_profiles_enabled,omitempty"`
	FeatureCWSRemoteConfigEnabled          bool     `json:"feature_cws_remote_config_enabled,omitempty"`
	FeatureCSMVMContainersEnabled          bool     `json:"feature_csm_vm_containers_enabled,omitempty"`
	FeatureCSMVMHostsEnabled               bool     `json:"feature_csm_vm_hosts_enabled,omitempty"`
	FeatureContainerImagesEnabled          bool     `json:"feature_container_images_enabled,omitempty"`
	FeatureProcessEnabled                  bool     `json:"feature_process_enabled,omitempty"`
	FeatureProcessesContainerEnabled       bool     `json:"feature_processes_container_enabled,omitempty"`
	FeatureProcessLanguageDetectionEnabled bool     `json:"feature_process_language_detection_enabled,omitempty"`
	FeatureNetworksEnabled                 bool     `json:"feature_networks_enabled,omitempty"`
	FeatureNetworksHTTPEnabled             bool     `json:"feature_networks_http_enabled,omitempty"`
	FeatureNetworksHTTPSEnabled            bool     `json:"feature_networks_https_enabled,omitempty"`
	FeatureLogsEnabled                     bool     `json:"feature_logs_enabled,omitempty"`
	FeatureCSPMEnabled                     bool     `json:"feature_cspm_enabled,omitempty"`
	FeatureAPMEnabled                      bool     `json:"feature_apm_enabled,omitempty"`
	FeatureRemoteConfigurationEnabled      bool     `json:"feature_remote_configuration_enabled,omitempty"`
	FeatureOTLPEnabled                     bool     `json:"feature_otlp_enabled,omitempty"`
	FeatureIMDSv2Enabled                   bool     `json:"feature_imdsv2_enabled,omitempty"`
	FeatureUSMEnabled                      bool     `json:"feature_usm_enabled,omitempty"`
	FeatureUSMKafkaEnabled                 bool     `json:"feature_usm_kafka_enabled,omitempty"`
	FeatureUSMJavaTLSEnabled               bool     `json:"feature_usm_java_tls_enabled,omitempty"`
	FeatureUSMGoTLSEnabled                 bool     `json:"feature_usm_go_tls_enabled,omitempty"`
	FeatureUSMHTTPByStatusCodeEnabled      bool     `json:"feature_usm_http_by_status_code_enabled,omitempty"`
	FeatureUSMHTTP2Enabled                 bool     `json:"feature_usm_http2_enabled,omitempty"`
	FeatureUSMIstioEnabled                 bool     `json:"feature_usm_istio_enabled,omitempty"`
	ECSFargateTaskARN                      string   `json:"ecs_fargate_task_arn,omitempty"`
	ECSFargateClusterName                  string   `json:"ecs_fargate_cluster_name,omitempty"`
	Hostname                               string   `json:"hostname,omitempty"`
	FleetPoliciesApplied                   []string `json:"fleet_policies_applied,omitempty"`
}

// Explicitly implement the JSONMarshaler interface
var (
	_ marshaler.JSONMarshaler = (*otelAgentPayload)(nil)
	_ marshaler.JSONMarshaler = (*agentPayload)(nil)
)

// Payload handles the JSON unmarshalling of the otel metadata payload
type otelAgentPayload struct {
	Hostname  string       `json:"hostname"`
	Timestamp int64        `json:"timestamp"`
	Metadata  OtelMetadata `json:"otel_metadata"`
	UUID      string       `json:"uuid"`
}

type agentPayload struct {
	Hostname  string        `json:"hostname"`
	Timestamp int64         `json:"timestamp"`
	Metadata  AgentMetadata `json:"agent_metadata"`
	UUID      string        `json:"uuid"`
}

type collectorModule struct {
	Type       string `json:"type"`
	Kind       string `json:"kind"`
	Gomod      string `json:"gomod"`
	Version    string `json:"version"`
	Configured bool   `json:"configured"`
}

type serviceComponent struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Kind            string `json:"kind"`
	Pipeline        string `json:"pipeline"`
	Gomod           string `json:"gomod"`
	Version         string `json:"version"`
	ComponentStatus string `json:"component_status"`
}

// moduleInfoJSON holds data on all modules in the collector
// It is built to make checking module info quicker when building active/configured components list
// (don't need to iterate through a whole list of modules, just do key/value pair in map)
type moduleInfoJSON struct {
	components map[string]collectorModule
}

func newModuleInfoJSON() *moduleInfoJSON {
	return &moduleInfoJSON{
		components: make(map[string]collectorModule),
	}
}

func (m *moduleInfoJSON) getKey(typeStr, kindStr string) string {
	return typeStr + ":" + kindStr
}

func (m *moduleInfoJSON) addComponent(comp collectorModule) {
	key := m.getKey(comp.Type, comp.Kind)
	m.components[key] = comp
	// We don't ever expect two modules to have the same type and kind
	// as collector would not be able to distinguish between them for configuration
	// and service/pipeline purposes.
}

func (m *moduleInfoJSON) getComponent(typeStr, kindStr string) (collectorModule, bool) {
	key := m.getKey(typeStr, kindStr)
	comp, ok := m.components[key]
	return comp, ok
}

func (m *moduleInfoJSON) MarshalJSON() ([]byte, error) {
	alias := struct {
		Components []collectorModule `json:"full_components"`
	}{
		Components: make([]collectorModule, 0o0, len(m.components)),
	}
	for _, comp := range m.components {
		alias.Components = append(alias.Components, comp)
	}
	return json.Marshal(alias)
}

type activeComponentsJSON struct {
	Components []serviceComponent `json:"active_components"`
}

// MarshalJSON serializes a Payload to JSON
func (p *otelAgentPayload) MarshalJSON() ([]byte, error) {
	type payloadAlias otelAgentPayload
	return json.Marshal((*payloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *otelAgentPayload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories otel payload any more, payload is too big for intake")
}

// MarshalJSON serializes a agentPayload to JSON
func (p *agentPayload) MarshalJSON() ([]byte, error) {
	type agentPayloadAlias agentPayload
	return json.Marshal((*agentPayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
func (p *agentPayload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
}
