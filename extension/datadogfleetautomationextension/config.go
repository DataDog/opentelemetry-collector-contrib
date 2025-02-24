// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
)

var (
	// ErrUnsetAPIKey is returned when the API key is not set.
	ErrUnsetAPIKey = datadogconfig.ErrUnsetAPIKey
	// ErrEmptyEndpoint is returned when endpoint is empty
	ErrEmptyEndpoint = datadogconfig.ErrEmptyEndpoint
	// ErrAPIKeyFormat is returned if API key contains invalid characters
	ErrAPIKeyFormat = datadogconfig.ErrAPIKeyFormat
	// NonHexRegex is a regex of characters that are always invalid in a Datadog API Key
	NonHexRegex = datadogconfig.NonHexRegex
)

var _ component.Config = (*Config)(nil)

const (
	// DefaultSite is the default site for the Datadog API.
	DefaultSite = datadogconfig.DefaultSite
	// NonHexChars is a regex of characters that are always invalid in a Datadog API key.
	NonHexChars = datadogconfig.NonHexChars
)

// Config contains the information necessary for enabling the Datadog Fleet
// Automation Extension.
type Config struct {
	confighttp.ClientConfig `mapstructure:",squash"`
	API                     APIConfig `mapstructure:"api"`
	// If Hostname is empty extension will use available system APIs and cloud provider endpoints.
	Hostname string `mapstructure:"hostname"`
}

// APIConfig contains the information necessary for configuring the Datadog API.
type APIConfig = datadogconfig.APIConfig

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.API.Site == "" {
		return ErrEmptyEndpoint
	}
	if c.API.Key == "" {
		return ErrUnsetAPIKey
	}
	invalidAPIKeyChars := NonHexRegex.FindAllString(string(c.API.Key), -1)
	if len(invalidAPIKeyChars) > 0 {
		return fmt.Errorf("%w: invalid characters: %s", ErrAPIKeyFormat, strings.Join(invalidAPIKeyChars, ", "))
	}
	return nil
}
