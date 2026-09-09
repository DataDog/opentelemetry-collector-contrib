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
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/sharedcomponent"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/metadata"
)

const defaultDeleteGracePeriod = 120 * time.Second

var consumerCapabilities = consumer.Capabilities{MutatesData: true}

type factory struct {
	processors *sharedcomponent.SharedComponents
}

// NewFactory returns a new factory for the docker attributes processor.
func NewFactory() processor.Factory {
	f := &factory{processors: sharedcomponent.NewSharedComponents()}
	return xprocessor.NewFactory(
		metadata.Type,
		createDefaultConfig,
		xprocessor.WithTraces(f.createTracesProcessor, metadata.TracesStability),
		xprocessor.WithMetrics(f.createMetricsProcessor, metadata.MetricsStability),
		xprocessor.WithLogs(f.createLogsProcessor, metadata.LogsStability),
		xprocessor.WithProfiles(f.createProfilesProcessor, metadata.ProfilesStability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Config:            *docker.NewDefaultConfig(),
		DeleteGracePeriod: defaultDeleteGracePeriod,
	}
}

func (f *factory) get(set processor.Settings, cfg *Config) (*sharedcomponent.SharedComponent, *dockerAttributesProcessor) {
	shared := f.processors.GetOrAdd(set.ID, func() component.Component {
		return newProcessor(set.TelemetrySettings, cfg)
	})
	return shared, shared.Unwrap().(*dockerAttributesProcessor)
}

func (f *factory) createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	shared, dp := f.get(set, cfg.(*Config))
	return processorhelper.NewTraces(
		ctx, set, cfg, next, dp.processTraces,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(shared.Start),
		processorhelper.WithShutdown(shared.Shutdown),
	)
}

func (f *factory) createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Metrics,
) (processor.Metrics, error) {
	shared, dp := f.get(set, cfg.(*Config))
	return processorhelper.NewMetrics(
		ctx, set, cfg, next, dp.processMetrics,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(shared.Start),
		processorhelper.WithShutdown(shared.Shutdown),
	)
}

func (f *factory) createLogsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Logs,
) (processor.Logs, error) {
	shared, dp := f.get(set, cfg.(*Config))
	return processorhelper.NewLogs(
		ctx, set, cfg, next, dp.processLogs,
		processorhelper.WithCapabilities(consumerCapabilities),
		processorhelper.WithStart(shared.Start),
		processorhelper.WithShutdown(shared.Shutdown),
	)
}

func (f *factory) createProfilesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next xconsumer.Profiles,
) (xprocessor.Profiles, error) {
	shared, dp := f.get(set, cfg.(*Config))
	return xprocessorhelper.NewProfiles(
		ctx, set, cfg, next, dp.processProfiles,
		xprocessorhelper.WithCapabilities(consumerCapabilities),
		xprocessorhelper.WithStart(shared.Start),
		xprocessorhelper.WithShutdown(shared.Shutdown),
	)
}
