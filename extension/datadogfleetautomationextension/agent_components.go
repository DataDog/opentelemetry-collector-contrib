// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"compress/gzip"
	"strings"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
)

func newLogComponent(set component.TelemetrySettings) corelog.Component {
	zlog := &datadog.Zaplogger{
		Logger: set.Logger,
	}
	return zlog
}

// The Forwarder sends the payloads to Datadog backend
func newForwarder(cfg coreconfig.Component, log corelog.Component) defaultforwarder.Forwarder {
	keysPerDomain := map[string][]string{"https://api." + cfg.GetString("site"): {cfg.GetString("api_key")}}
	forwarderOptions := defaultforwarder.NewOptions(cfg, log, keysPerDomain)
	forwarderOptions.DisableAPIKeyChecking = true
	return defaultforwarder.NewDefaultForwarder(cfg, log, forwarderOptions)
}

// create compressor with Gzip strategy, best compression
func newCompressor() compression.Compressor {
	return selector.NewCompressor(compression.GzipKind, gzip.BestCompression)
}

// The Serializer serializes the payloads prior to being forwarded by the Forwarder
func newSerializer(fwd defaultforwarder.Forwarder, cmp compression.Compressor, cfg coreconfig.Component, logger corelog.Component, hostname string) *serializer.Serializer {
	return serializer.NewSerializer(fwd, nil, cmp, cfg, logger, hostname)
}

// Config component from datadog-agent is required to use forwarder and serializer components
func newConfigComponent(set component.TelemetrySettings, cfg *Config) coreconfig.Component {
	pkgconfig := viperconfig.NewConfig("DD", "DD", strings.NewReplacer(".", "_"))

	// Set the API Key
	pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
	pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("log_level", set.Logger.Level().String(), pkgconfigmodel.SourceFile)
	// Set values for serializer
	pkgconfig.Set("enable_payloads.events", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("enable_payloads.json_to_v1_intake", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("enable_sketch_stream_payload_serialization", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_apikey_validation_interval", 60, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_num_workers", 1, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logging_frequency", 2, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_factor", 2, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_base", 2, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_backoff_max", 64, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("forwarder_recovery_interval", 2, pkgconfigmodel.SourceDefault)
	return pkgconfig
}
