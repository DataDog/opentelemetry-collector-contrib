// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package httpserver // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/httpserver"

import "go.opentelemetry.io/collector/config/confighttp"

const (
	DefaultServerPort = 8088
)

// Config contains the v2 config for the http metadata service
type Config struct {
	confighttp.ServerConfig `mapstructure:",squash"`
	Enabled                 bool   `mapstructure:"enabled"`
	Path                    string `mapstructure:"path"`
}
