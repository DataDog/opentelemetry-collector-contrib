// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension/extensiontest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/httpserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/testutil"
)

func TestFactory_CreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.Equal(t, &Config{
		ClientConfig: confighttp.NewDefaultClientConfig(),
		API: APIConfig{
			Site: defaultSite,
		},
		HTTPConfig: &httpserver.Config{
			ServerConfig: confighttp.ServerConfig{
				Endpoint: testutil.EndpointForPort(httpserver.DefaultServerPort),
			},
			Enabled: true,
			Path:    "/metadata",
		},
	}, cfg)
}

func TestFactory_Create(t *testing.T) {
	cfg := createDefaultConfig().(*Config)

	ext, err := create(context.Background(), extensiontest.NewNopSettings(typ), cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
}

func TestFactory_NewFactory(t *testing.T) {
	factory := NewFactory()

	assert.Equal(t, metadata.Type, factory.Type())
	assert.NotNil(t, factory.CreateDefaultConfig)
	assert.NotNil(t, factory.Create)

	// Test CreateDefaultConfig
	defaultConfig := factory.CreateDefaultConfig()
	expectedConfig := &Config{
		ClientConfig: confighttp.NewDefaultClientConfig(),
		API: APIConfig{
			Site: defaultSite,
		},
		HTTPConfig: &httpserver.Config{
			ServerConfig: confighttp.ServerConfig{
				Endpoint: testutil.EndpointForPort(httpserver.DefaultServerPort),
			},
			Enabled: true,
			Path:    "/metadata",
		},
	}
	assert.Equal(t, expectedConfig, defaultConfig)

	// Test CreateExtension
	ext, err := factory.Create(context.Background(), extensiontest.NewNopSettings(metadata.Type), defaultConfig)
	assert.NoError(t, err)
	assert.NotNil(t, ext)

	expectedConfig.API.FailOnInvalidKey = true
	expectedConfig.API.Key = "bad-key"
	ext, err = factory.Create(context.Background(), extensiontest.NewNopSettings(metadata.Type), expectedConfig)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "API Key validation failed")
	assert.Nil(t, ext)
}
