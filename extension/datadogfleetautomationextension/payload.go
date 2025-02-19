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

type HostMetadata struct {
	CPUArchitecture              string  `json:"cpu_architecture"`
	CPUCacheSize                 int     `json:"cpu_cache_size"`
	CPUCores                     int     `json:"cpu_cores"`
	CPUFamily                    string  `json:"cpu_family"`
	CPUFrequency                 float64 `json:"cpu_frequency"`
	CPULogicalProcessors         int     `json:"cpu_logical_processors"`
	CPUModel                     string  `json:"cpu_model"`
	CPUModelID                   string  `json:"cpu_model_id"`
	CPUStepping                  string  `json:"cpu_stepping"`
	CPUVendor                    string  `json:"cpu_vendor"`
	KernelName                   string  `json:"kernel_name"`
	KernelRelease                string  `json:"kernel_release"`
	KernelVersion                string  `json:"kernel_version"`
	OS                           string  `json:"os"`
	OSVersion                    string  `json:"os_version"`
	MemorySwapTotalKB            int     `json:"memory_swap_total_kb"`
	MemoryTotalKB                int     `json:"memory_total_kb"`
	IPAddress                    string  `json:"ip_address"`
	IPv6Address                  string  `json:"ipv6_address"`
	MACAddress                   string  `json:"mac_address"`
	AgentVersion                 string  `json:"agent_version"`
	CloudProvider                string  `json:"cloud_provider"`
	CloudProviderSource          string  `json:"cloud_provider_source"`
	CloudProviderAccountID       string  `json:"cloud_provider_account_id"`
	CloudProviderHostID          string  `json:"cloud_provider_host_id"`
	HypervisorGuestUUID          string  `json:"hypervisor_guest_uuid"`
	DMIProductUUID               string  `json:"dmi_product_uuid"`
	DMIAssetTag                  string  `json:"dmi_board_asset_tag"`
	DMIAssetVendor               string  `json:"dmi_board_vendor"`
	LinuxPackageSigningEnabled   bool    `json:"linux_package_signing_enabled"`
	RPMGlobalRepoGPGCheckEnabled bool    `json:"rpm_global_repo_gpg_check_enabled"`
}

type CombinedPayload struct {
	MetadataPayload metadataPayload `json:"metadata_payload"`
	AgentPayload    agentPayload    `json:"agent_payload"`
	OtelPayload     payload         `json:"otel_payload"`
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

// metadataPayload handles the JSON unmarshalling of the host metadata payload
type metadataPayload struct {
	Hostname  string       `json:"hostname"`
	Timestamp int64        `json:"timestamp"`
	Metadata  HostMetadata `json:"host_metadata"`
	UUID      string       `json:"uuid"`
}

type agentPayload struct {
	Hostname  string        `json:"hostname"`
	Timestamp int64         `json:"timestamp"`
	Metadata  AgentMetadata `json:"agent_metadata"`
	UUID      string        `json:"uuid"`
}

type collectorComponent struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Module          string `json:"module"`
	Version         string `json:"version"`
	Enabled         bool   `json:"enabled"`
	ComponentStatus string `json:"component_status"`
}

type moduleInfoJSON struct {
	Components []collectorComponent `json:"components"`
}

// MarshalJSON serializes a metadataPayload to JSON
func (p *metadataPayload) MarshalJSON() ([]byte, error) {
	type metadataPayloadAlias metadataPayload
	return json.Marshal((*metadataPayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler
func (p *metadataPayload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
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
