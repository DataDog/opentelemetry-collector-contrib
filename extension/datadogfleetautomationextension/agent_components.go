// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"runtime"
	"strings"

	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/comp/core/config"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
)

func newLogComponent(set component.TelemetrySettings) corelog.Component {
	zlog := &zaplogger{
		logger: set.Logger,
	}
	return zlog
}

func newForwarder(cfg config.Component, log log.Component) *defaultforwarder.DefaultForwarder {
	// fmt.Println("forwarder api_key: ", string(cfg.GetString("api_key")))
	keysPerDomain := map[string][]string{"https://api." + cfg.GetString("site"): {string(cfg.GetString("api_key"))}}
	return defaultforwarder.NewDefaultForwarder(cfg, log, defaultforwarder.NewOptions(cfg, log, keysPerDomain))
}

func newCompressor() compression.Compressor {
	return selector.NewCompressor(compression.NoneKind, 0)
}

func newSerializer(fwd *defaultforwarder.DefaultForwarder, cmp compression.Compressor, cfg config.Component) *serializer.Serializer {
	return serializer.NewSerializer(fwd, nil, cmp, cfg, metadata.Type.String())
}

func newConfigComponent(set component.TelemetrySettings, cfg *Config) coreconfig.Component {
	pkgconfig := pkgconfigmodel.NewConfig("DD", "DD", strings.NewReplacer(".", "_"))

	// Set the API Key
	// fmt.Println("cfg api_key: ", string(cfg.API.Key))
	pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
	pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("log_level", set.Logger.Level().String(), pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.auditor_ttl", pkgconfigsetup.DefaultAuditorTTL, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.batch_max_content_size", pkgconfigsetup.DefaultBatchMaxContentSize, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.batch_max_size", pkgconfigsetup.DefaultBatchMaxSize, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.force_use_http", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.input_chan_size", pkgconfigsetup.DefaultInputChanSize, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.max_message_size_bytes", pkgconfigsetup.DefaultMaxMessageSizeBytes, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.run_path", "/opt/datadog-agent/run", pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.sender_backoff_factor", pkgconfigsetup.DefaultLogsSenderBackoffFactor, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.sender_backoff_base", pkgconfigsetup.DefaultLogsSenderBackoffBase, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.sender_backoff_max", pkgconfigsetup.DefaultLogsSenderBackoffMax, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.sender_recovery_interval", pkgconfigsetup.DefaultForwarderRecoveryInterval, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.stop_grace_period", 30, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.use_v2_api", true, pkgconfigmodel.SourceDefault)
	pkgconfig.SetKnown("logs_config.dev_mode_no_ssl")
	// add logs config pipelines config value, see https://github.com/DataDog/datadog-agent/pull/31190
	logsPipelines := min(4, runtime.GOMAXPROCS(0))
	pkgconfig.Set("logs_config.pipelines", logsPipelines, pkgconfigmodel.SourceDefault)
	// Set values for serializer
	pkgconfig.Set("enable_payloads.events", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("enable_payloads.json_to_v1_intake", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("enable_sketch_stream_payload_serialization", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_apikey_validation_interval", 60, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_num_workers", 1, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logging_frequency", 1, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_factor", 2, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_base", 2, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_max", 64, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_recovery_interval", 2, pkgconfigmodel.SourceDefault)
	return pkgconfig
}
