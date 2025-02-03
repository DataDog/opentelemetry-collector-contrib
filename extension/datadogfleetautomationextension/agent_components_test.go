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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

	// Assert that the returned component is of type *zaplogger
	zlog, ok := logComponent.(*zaplogger)
	assert.True(t, ok, "Expected logComponent to be of type *zaplogger")

	// Assert that the logger is correctly set
	assert.Equal(t, logger, zlog.logger)
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
	assert.Equal(t, pkgconfigsetup.DefaultAuditorTTL, configComponent.GetInt("logs_config.auditor_ttl"))
	assert.Equal(t, pkgconfigsetup.DefaultBatchMaxContentSize, configComponent.GetInt("logs_config.batch_max_content_size"))
	assert.Equal(t, pkgconfigsetup.DefaultBatchMaxSize, configComponent.GetInt("logs_config.batch_max_size"))
	assert.Equal(t, true, configComponent.GetBool("logs_config.force_use_http"))
	assert.Equal(t, pkgconfigsetup.DefaultInputChanSize, configComponent.GetInt("logs_config.input_chan_size"))
	assert.Equal(t, pkgconfigsetup.DefaultMaxMessageSizeBytes, configComponent.GetInt("logs_config.max_message_size_bytes"))
	assert.Equal(t, "/opt/datadog-agent/run", configComponent.GetString("logs_config.run_path"))
	assert.Equal(t, pkgconfigsetup.DefaultLogsSenderBackoffFactor, configComponent.GetFloat64("logs_config.sender_backoff_factor"))
	assert.Equal(t, pkgconfigsetup.DefaultLogsSenderBackoffBase, configComponent.GetFloat64("logs_config.sender_backoff_base"))
	assert.Equal(t, pkgconfigsetup.DefaultLogsSenderBackoffMax, configComponent.GetFloat64("logs_config.sender_backoff_max"))
	assert.Equal(t, pkgconfigsetup.DefaultForwarderRecoveryInterval, configComponent.GetInt("logs_config.sender_recovery_interval"))
	assert.Equal(t, 30, configComponent.GetInt("logs_config.stop_grace_period"))
	assert.Equal(t, true, configComponent.GetBool("logs_config.use_v2_api"))
	assert.Equal(t, true, configComponent.GetBool("enable_payloads.events"))
	assert.Equal(t, true, configComponent.GetBool("enable_payloads.json_to_v1_intake"))
	assert.Equal(t, true, configComponent.GetBool("enable_sketch_stream_payload_serialization"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
