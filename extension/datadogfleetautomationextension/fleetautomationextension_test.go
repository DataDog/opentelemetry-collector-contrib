// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap/zaptest"
)

func Test_NotifyConfig(t *testing.T) {
	// Create a simple confmap.Conf
	configData := map[string]any{
		"service": map[string]any{
			"pipelines": map[string]any{
				"traces": map[string]any{
					"receivers": []any{"otlp"},
					"exporters": []any{"debug"},
				},
			},
		},
	}
	conf := confmap.NewFromStringMap(configData)

	// Create a background context
	ctx := context.Background()

	// Create a logger for testing
	logger := zaptest.NewLogger(t)

	// Create telemetry settings with the test logger
	telemetry := componenttest.NewNopTelemetrySettings()
	telemetry.Logger = logger

	faExt := newExtension(&Config{}, telemetry)
	err := faExt.NotifyConfig(ctx, conf)
	assert.NoError(t, err)

	// Verify that the configuration is correctly set
	assert.Equal(t, conf, faExt.collectorConfig)
}
