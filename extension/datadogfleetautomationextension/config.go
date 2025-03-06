// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	// DefaultReporterPeriod is the default amount of time between sending fleet automation payloads to Datadog.
	DefaultReporterPeriod = 20 * time.Minute
)

// Config contains the information necessary for enabling the Datadog Fleet
// Automation Extension.
type Config struct {
	confighttp.ClientConfig `mapstructure:",squash"`
	API                     APIConfig `mapstructure:"api"`
	// If Hostname is empty extension will use available system APIs and cloud provider endpoints.
	Hostname string `mapstructure:"hostname"`
	// ReporterPeriod sets the amount of time between sending fleet automation payloads to Datadog.
	ReporterPeriod time.Duration `mapstructure:"reporter_period"`
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
	if c.ReporterPeriod < 5*time.Minute {
		return errors.New("reporter_period must be 5 minutes or higher")
	}
	return nil
}
