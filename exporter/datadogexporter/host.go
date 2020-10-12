// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datadogexporter

import (
	"fmt"
	"os"
)

const (
	opentelemetryFlavor  = "opentelemetry-collector"
	opentelemetryVersion = "alpha"
)

var (
	userAgent = fmt.Sprintf("%s/%s", opentelemetryFlavor, opentelemetryVersion)
)

// GetHost gets the hostname according to configuration.
// It gets the configuration hostname and if
// not available it relies on the OS hostname
func GetHost(cfg *Config) *string {
	if cfg.TagsConfig.Hostname != "" {
		return &cfg.TagsConfig.Hostname
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return &host
}

// hostMetadata includes metadata about the host tags,
// host aliases and identifies the host as an OpenTelemetry host
type hostMetadata struct {
	// Meta includes metadata about the host.
	Meta *meta `json:"meta"`

	// InternalHostname is the canonical hostname
	InternalHostname string `json:"internalHostname"`

	// Version is the OpenTelemetry Collector version.
	// This is used for correctly identifying the Collector in the backend,
	// and for telemetry purposes.
	Version string `json:"otel_version"`

	// Flavor is always set to "opentelemetry-collector".
	// It is used for telemetry purposes in the backend.
	Flavor string `json:"agent-flavor"`

	// Tags includes the host tags
	Tags *hostTags `json:"host-tags"`
}

// hostTags are the host tags.
// Currently only system (configuration) tags are considered.
type hostTags struct {
	// System are host tags set in the configuration
	System []string `json:"system,omitempty"`
}

// meta includes metadata about the host aliases
type meta struct {
	// InstanceID is the EC2 instance id the Collector is running on, if available
	InstanceID string `json:"instance-id,omitempty"`

	// EC2Hostname is the hostname from the EC2 metadata API
	EC2Hostname string `json:"ec2-hostname,omitempty"`

	// Hostname is the canonical hostname
	Hostname string `json:"hostname"`

	// SocketHostname is the OS hostname
	SocketHostname string `json:"socket-hostname"`

	// HostAliases are other available host names
	HostAliases []string `json:"host-aliases,omitempty"`
}

func getHostMetadata(cfg *Config) hostMetadata {
	host := *GetHost(cfg)
	return hostMetadata{
		InternalHostname: host,
		Flavor:           opentelemetryFlavor,
		Version:          opentelemetryVersion,
		Tags:             &hostTags{cfg.TagsConfig.GetTags()},
		Meta: &meta{
			Hostname: host,
		},
	}
}
