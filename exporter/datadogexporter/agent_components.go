// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"fmt"
	//"fmt"
	"runtime"
	"strings"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	//"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	//compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	//"github.com/DataDog/datadog-agent/pkg/serializer"
	"go.opentelemetry.io/collector/component"
	//"go.uber.org/fx"
	//"go.uber.org/fx/fxevent"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
)

func newLogComponent(set component.TelemetrySettings) corelog.Component {
	zlog := &zaplogger{
		logger: set.Logger,
	}
	return zlog
}

func newConfigComponent(set component.TelemetrySettings, cfg *Config) coreconfig.Component {
	pkgconfig := pkgconfigmodel.NewConfig("DD", "DD", strings.NewReplacer(".", "_"))

	// Set the API Key
	pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
	pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("log_level", set.Logger.Level().String(), pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.batch_wait", cfg.Logs.BatchWait, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.use_compression", cfg.Logs.UseCompression, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.compression_level", cfg.Logs.CompressionLevel, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.logs_dd_url", cfg.Logs.TCPAddrConfig.Endpoint, pkgconfigmodel.SourceFile)
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
	return pkgconfig
}

func newMetricSerializer(set component.TelemetrySettings, cfg *Config, sourceProvider source.Provider) (*serializer.Serializer, error) {
	var f defaultforwarder.Component
	var c coreconfig.Component
	var s *serializer.Serializer
	app := fx.New(
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),
		fx.Supply(set.Logger),
		fx.Supply(set),
		fx.Supply(cfg),
		fx.Provide(newLogComponent),
		fx.Provide(newConfigComponent),

		//fx.Provide(func(c coreconfig.Component, l corelog.Component) (defaultforwarder.Params, error) {
		//	return defaultforwarder.NewParams()	, nil
		//}),
		// casts the defaultforwarder.Component to a defaultforwarder.Forwarder
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		// this is the hostname argument for serializer.NewSerializer
		// this should probably be wrapped by a type
		fx.Provide(func() string {
			s, err := sourceProvider.Source(context.TODO())
			if err != nil {
				return ""
			}
			return s.Identifier
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Provide(serializer.NewSerializer),
		//fx.Provide(strategy.NewZlibStrategy),
		// this doesn't let us switch impls.........
		metricscompressionfx.Module(),
		// casts the metricscompression.Component to a compression.Compressor
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		//fx.Provide(func(s *strategy.ZlibStrategy) compression.Component {
		//	return s
		//}),
		defaultforwarder.Module(defaultforwarder.NewParams()),
		fx.Populate(&f),
		fx.Populate(&c),
		fx.Populate(&s),
	)
	fmt.Printf("### done with app\n")
	if err := app.Err(); err != nil {
		return nil, err
	}
	go func() {
		forwarder := f.(*defaultforwarder.DefaultForwarder)
		err := forwarder.Start()
		if err != nil {
			fmt.Printf("### error starting forwarder: %s\n", err)
		}
	}()
	return s, nil
}

type orchestratorinterfaceimpl struct {
	f defaultforwarder.Forwarder
}

func newOrchestratorinterfaceimpl(f defaultforwarder.Forwarder) orchestratorinterface.Component {
	return &orchestratorinterfaceimpl{
		f: f,
	}
}

func (o *orchestratorinterfaceimpl) Get() (defaultforwarder.Forwarder, bool) {
	return o.f, true
}

func newSerializerExporter(s serializer.Serializer, set exporter.Settings, cfg *Config, statsOut chan []byte) (*serializerexporter.Exporter, error) {
	hostGetter := func(_ context.Context) (string, error) {
		return "", nil
	}
	// todo(ankit) quadruple check all these settings
	exporterConfig := &serializerexporter.ExporterConfig{
		Metrics: serializerexporter.MetricsConfig{
			Metrics: datadogconfig.MetricsConfig{
				DeltaTTL: cfg.Metrics.DeltaTTL,
				ExporterConfig: datadogconfig.MetricsExporterConfig{
					ResourceAttributesAsTags:           cfg.Metrics.ExporterConfig.ResourceAttributesAsTags,
					InstrumentationScopeMetadataAsTags: cfg.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags,
				},
				HistConfig: datadogconfig.HistogramConfig{
					Mode: cfg.Metrics.HistConfig.Mode,
				},
			},
		},
	}
	exporterConfig.QueueConfig = cfg.QueueSettings
	exporterConfig.TimeoutConfig = exporterhelper.TimeoutConfig{
		Timeout: cfg.Timeout,
	}

	// TODO: Ideally the attributes translator would be created once and reused
	// across all signals. This would need unifying the logsagent and serializer
	// exporters into a single exporter.
	attributesTranslator, err := attributes.NewTranslator(set.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	fmt.Printf("### created attributes translator\n")
	newExp, err := serializerexporter.NewExporter(set.TelemetrySettings, attributesTranslator, &s, exporterConfig, &tagEnricher{}, hostGetter, statsOut)
	if err != nil {
		return nil, err
	}

	return newExp, nil
}

type tagEnricher struct{}

func (t *tagEnricher) SetCardinality(_ string) (err error) {
	return nil
}

// Enrich of a given dimension.
func (t *tagEnricher) Enrich(_ context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)
	return enrichedTags
}