// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper"
	"go.opentelemetry.io/collector/processor/xprocessor"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/metadata"
)

const defaultDeleteGracePeriod = 120 * time.Second

var consumerCapabilities = consumer.Capabilities{MutatesData: true}

// NewFactory returns a new factory for the docker attributes processor.
func NewFactory() processor.Factory {
	return xprocessor.NewFactory(
		metadata.Type,
		createDefaultConfig,
		xprocessor.WithTraces(createTracesProcessor, metadata.TracesStability),
		xprocessor.WithMetrics(createMetricsProcessor, metadata.MetricsStability),
		xprocessor.WithLogs(createLogsProcessor, metadata.LogsStability),
		xprocessor.WithProfiles(createProfilesProcessor, metadata.ProfilesStability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Config:            *docker.NewDefaultConfig(),
		DeleteGracePeriod: defaultDeleteGracePeriod,
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	dp := newProcessor(set.TelemetrySettings, cfg.(*Config))
	return processorhelper.NewTraces(
		ctx, set, cfg, next, dp.processTraces,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(dp.Start),
		processorhelper.WithShutdown(dp.Shutdown),
	)
}

func createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Metrics,
) (processor.Metrics, error) {
	dp := newProcessor(set.TelemetrySettings, cfg.(*Config))
	return processorhelper.NewMetrics(
		ctx, set, cfg, next, dp.processMetrics,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(dp.Start),
		processorhelper.WithShutdown(dp.Shutdown),
	)
}

func createLogsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Logs,
) (processor.Logs, error) {
	dp := newProcessor(set.TelemetrySettings, cfg.(*Config))
	return processorhelper.NewLogs(
		ctx, set, cfg, next, dp.processLogs,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(dp.Start),
		processorhelper.WithShutdown(dp.Shutdown),
	)
}

func createProfilesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next xconsumer.Profiles,
) (xprocessor.Profiles, error) {
	dp := newProcessor(set.TelemetrySettings, cfg.(*Config))
	return xprocessorhelper.NewProfiles(
		ctx, set, cfg, next, dp.processProfiles,
		xprocessorhelper.WithCapabilities(consumerCapabilities),
		xprocessorhelper.WithStart(dp.Start),
		xprocessorhelper.WithShutdown(dp.Shutdown),
	)
}
