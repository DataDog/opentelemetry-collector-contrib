// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
)

func TestNewLogComponent(t *testing.T) {
	// Create a zap logger for testing
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger, err := config.Build()
	if err != nil {
		t.Fatalf("Failed to build logger: %v", err)
	}

	// Create a TelemetrySettings with the test logger
	telemetrySettings := component.TelemetrySettings{
		Logger: logger,
	}

	// Call newLogComponent
	logComponent := newLogComponent(telemetrySettings)

	// Assert that the returned component is not nil
	assert.NotNil(t, logComponent)

	// Assert that the returned component is of type *datadog.Zaplogger
	zlog, ok := logComponent.(*datadog.Zaplogger)
	assert.True(t, ok, "Expected logComponent to be of type *datadog.Zaplogger")

	// Assert that the logger is correctly set
	assert.Equal(t, logger, zlog.Logger)
}

func TestNewConfigComponent(t *testing.T) {
	// Create a zap logger for testing
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger, err := config.Build()
	if err != nil {
		t.Fatalf("Failed to build logger: %v", err)
	}

	// Create a TelemetrySettings with the test logger
	telemetrySettings := component.TelemetrySettings{
		Logger: logger,
	}

	// Create a test Config
	cfg := &Config{}
	cfg.API.Key = "test-api-key"
	cfg.API.Site = "test-site"

	// Call newConfigComponent
	configComponent := newConfigComponent(telemetrySettings, cfg)

	// Assert that the returned component is not nil
	assert.NotNil(t, configComponent)

	// Assert that the configuration values are set correctly
	assert.Equal(t, "test-api-key", configComponent.GetString("api_key"))
	assert.Equal(t, "test-site", configComponent.GetString("site"))
	assert.Equal(t, true, configComponent.GetBool("logs_enabled"))
	assert.Equal(t, "info", configComponent.GetString("log_level"))
	assert.Equal(t, true, configComponent.GetBool("enable_payloads.events"))
	assert.Equal(t, true, configComponent.GetBool("enable_payloads.json_to_v1_intake"))
	assert.Equal(t, true, configComponent.GetBool("enable_sketch_stream_payload_serialization"))
	assert.Equal(t, 60, configComponent.GetInt("forwarder_apikey_validation_interval"))
	assert.Equal(t, 1, configComponent.GetInt("forwarder_num_workers"))
	assert.Equal(t, 2, configComponent.GetInt("logging_frequency"))
	assert.Equal(t, 2, configComponent.GetInt("forwarder_backoff_factor"))
	assert.Equal(t, 2, configComponent.GetInt("forwarder_backoff_base"))
	assert.Equal(t, 64, configComponent.GetInt("forwarder_backoff_max"))
	assert.Equal(t, 2, configComponent.GetInt("forwarder_recovery_interval"))
}
