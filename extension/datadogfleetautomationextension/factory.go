// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/agentcomponents"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/hostmetadata"
)

// NewFactory creates a factory for the Datadog Fleet Automation extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		metadata.Type,
		createDefaultConfig,
		create,
		metadata.ExtensionStability,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		ClientConfig: confighttp.NewDefaultClientConfig(),
		API: APIConfig{
			Site: defaultSite,
		},
	}
}

func create(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	apiKeyValidator := clientutil.ValidateAPIKey
	sourceProviderGetter := hostmetadata.GetSourceProvider
	faExt, err := newExtension(ctx, cfg.(*Config), set, apiKeyValidator, sourceProviderGetter, agentcomponents.NewForwarder)
	if err != nil {
		return nil, err
	}
	return faExt, nil
}
