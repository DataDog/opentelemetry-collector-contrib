// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

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
	AgentPayload agentPayload `json:"agent_payload"`
	OtelPayload  payload      `json:"otel_payload"`
}

type AgentMetadata struct {
	AgentVersion                           string   `json:"agent_version"`
	AgentStartupTimeMs                     int64    `json:"agent_startup_time_ms"`
	AgentFlavor                            string   `json:"flavor"`
	ConfigAPMDDUrl                         string   `json:"config_apm_dd_url"`
	ConfigDDUrl                            string   `json:"config_dd_url"`
	ConfigSite                             string   `json:"config_site"`
	ConfigLogsDDUrl                        string   `json:"config_logs_dd_url"`
	ConfigLogsSocks5ProxyAddress           string   `json:"config_logs_socks5_proxy_address"`
	ConfigNoProxy                          []string `json:"config_no_proxy"`
	ConfigProcessDDUrl                     string   `json:"config_process_dd_url"`
	ConfigProxyHTTP                        string   `json:"config_proxy_http"`
	ConfigProxyHTTPS                       string   `json:"config_proxy_https"`
	ConfigEKSFargate                       bool     `json:"config_eks_fargate"`
	InstallMethodTool                      string   `json:"install_method_tool"`
	InstallMethodToolVersion               string   `json:"install_method_tool_version"`
	InstallMethodInstallerVersion          string   `json:"install_method_installer_version"`
	LogsTransport                          string   `json:"logs_transport"`
	FeatureFIPSEnabled                     bool     `json:"feature_fips_enabled"`
	FeatureCWSEnabled                      bool     `json:"feature_cws_enabled"`
	FeatureCWSNetworkEnabled               bool     `json:"feature_cws_network_enabled"`
	FeatureCWSSecurityProfilesEnabled      bool     `json:"feature_cws_security_profiles_enabled"`
	FeatureCWSRemoteConfigEnabled          bool     `json:"feature_cws_remote_config_enabled"`
	FeatureCSMVMContainersEnabled          bool     `json:"feature_csm_vm_containers_enabled"`
	FeatureCSMVMHostsEnabled               bool     `json:"feature_csm_vm_hosts_enabled"`
	FeatureContainerImagesEnabled          bool     `json:"feature_container_images_enabled"`
	FeatureProcessEnabled                  bool     `json:"feature_process_enabled"`
	FeatureProcessesContainerEnabled       bool     `json:"feature_processes_container_enabled"`
	FeatureProcessLanguageDetectionEnabled bool     `json:"feature_process_language_detection_enabled"`
	FeatureNetworksEnabled                 bool     `json:"feature_networks_enabled"`
	FeatureNetworksHTTPEnabled             bool     `json:"feature_networks_http_enabled"`
	FeatureNetworksHTTPSEnabled            bool     `json:"feature_networks_https_enabled"`
	FeatureLogsEnabled                     bool     `json:"feature_logs_enabled"`
	FeatureCSPMEnabled                     bool     `json:"feature_cspm_enabled"`
	FeatureAPMEnabled                      bool     `json:"feature_apm_enabled"`
	FeatureRemoteConfigurationEnabled      bool     `json:"feature_remote_configuration_enabled"`
	FeatureOTLPEnabled                     bool     `json:"feature_otlp_enabled"`
	FeatureIMDSv2Enabled                   bool     `json:"feature_imdsv2_enabled"`
	FeatureUSMEnabled                      bool     `json:"feature_usm_enabled"`
	FeatureUSMKafkaEnabled                 bool     `json:"feature_usm_kafka_enabled"`
	FeatureUSMJavaTLSEnabled               bool     `json:"feature_usm_java_tls_enabled"`
	FeatureUSMGoTLSEnabled                 bool     `json:"feature_usm_go_tls_enabled"`
	FeatureUSMHTTPByStatusCodeEnabled      bool     `json:"feature_usm_http_by_status_code_enabled"`
	FeatureUSMHTTP2Enabled                 bool     `json:"feature_usm_http2_enabled"`
	FeatureUSMIstioEnabled                 bool     `json:"feature_usm_istio_enabled"`
	ECSFargateTaskARN                      string   `json:"ecs_fargate_task_arn"`
	ECSFargateClusterName                  string   `json:"ecs_fargate_cluster_name"`
	Hostname                               string   `json:"hostname"`
	FleetPoliciesApplied                   []string `json:"fleet_policies_applied"`
}

var _ marshaler.JSONMarshaler = (*payload)(nil)

// Payload handles the JSON unmarshalling of the otel metadata payload
type payload struct {
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
	Type              string `json:"type"`
	Kind              string `json:"kind"`
	Gomod             string `json:"gomod"`
	Version           string `json:"version"`
	IncludedInService bool   `json:"included_in_service"`
}

type serviceComponent struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Type            string `json:"type"`
	Kind            string `json:"kind"`
	Gomod           string `json:"gomod"`
	Version         string `json:"version"`
	ComponentStatus string `json:"component_status"`
}

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
		Components: make([]collectorModule, 00, len(m.components)),
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
func (p *payload) MarshalJSON() ([]byte, error) {
	type payloadAlias payload
	return json.Marshal((*payloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
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
