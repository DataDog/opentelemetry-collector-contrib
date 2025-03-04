// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/hostmetadata"
)

// NewFactory creates a factory for the Datadog Fleet Automation Extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(metadata.Type, createDefaultConfig, create, metadata.ExtensionStability)
}

func createDefaultConfig() component.Config {
	return &Config{
		ClientConfig: confighttp.NewDefaultClientConfig(),
		API: APIConfig{
			Site: DefaultSite,
		},
		ReporterPeriod: defaultReporterPeriod,
	}
}

func create(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	apiKeyValidator := clientutil.ValidateAPIKey
	sourceProviderGetter := hostmetadata.GetSourceProvider
	ext, err := newExtension(ctx, cfg.(*Config), set, apiKeyValidator, sourceProviderGetter, newForwarder)
	if err != nil {
		return nil, err
	}
	// componentChecker interface allows certain component check modules to be mocked/overwritten on testing
	componentChecker := &defaultComponentChecker{extension: ext}
	ext.componentChecker = componentChecker
	return ext, nil
}
