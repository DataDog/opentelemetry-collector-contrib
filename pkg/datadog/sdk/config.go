// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package sdk hosts native SDK instrumentation telemetry in a Collector.
package sdk // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/sdk"

import (
	"errors"
	"net"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
)

type APIConfig struct {
	Key  configopaque.String
	Site string
}

// Config holds resolved SDK listener and forwarding settings.
type Config struct {
	ServerConfig             confighttp.ServerConfig
	InstrumentationTelemetry InstrumentationTelemetryConfig
	API                      APIConfig
	Backend                  confighttp.ClientConfig
	Version                  string
}

func DefaultConfig() *Config {
	server := confighttp.NewDefaultServerConfig()
	server.NetAddr.Endpoint = "127.0.0.1:8234"
	server.ReadHeaderTimeout = 5 * time.Second
	server.ReadTimeout = 10 * time.Second
	server.MaxRequestBodySize = 1 << 20
	return &Config{ServerConfig: server, API: APIConfig{Site: "datadoghq.com"}, Backend: confighttp.NewDefaultClientConfig(), InstrumentationTelemetry: DefaultInstrumentationTelemetryConfig()}
}

var sitePattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func ValidateConfig(c *Config) error {
	if strings.TrimSpace(string(c.API.Key)) == "" || !sitePattern.MatchString(c.API.Site) {
		return errors.New("SDK forwarding requires a Collector api.key and a valid api.site")
	}
	b := c.Backend
	if b.TLS.Insecure || b.TLS.InsecureSkipVerify || b.Auth.HasValue() || len(b.Headers) > 0 || len(b.Middlewares) > 0 || b.Compression.IsCompressed() {
		return errors.New("SDK forwarding uses api.key and requires verified backend TLS without custom headers, middleware or compression")
	}
	if c.ServerConfig.MaxRequestBodySize <= 0 {
		return errors.New("SDK listener max_request_body_size must be positive")
	}
	if _, _, err := net.SplitHostPort(c.ServerConfig.NetAddr.Endpoint); err != nil {
		return errors.New("sdk_endpoint must be a host:port address")
	}
	if c.InstrumentationTelemetry.Enabled {
		return c.InstrumentationTelemetry.validate(c.API.Site)
	}
	return nil
}
