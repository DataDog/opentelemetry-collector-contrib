// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension/extensiontest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
)

func TestFactory_CreateDefaultConfig(t *testing.T) {
	expectedConfig := &Config{
		ClientConfig: confighttp.NewDefaultClientConfig(),
		API: APIConfig{
			Site: defaultSite,
		},
	}
	cfg := createDefaultConfig()
	assert.Equal(t, expectedConfig, cfg)

	require.NoError(t, componenttest.CheckConfigStruct(cfg))
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
