// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/metrics"

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"go.opentelemetry.io/collector/component"
	zorkian "gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/metrics/sketches"
)

var _ translator.Consumer = (*Consumer)(nil)
var _ translator.HostConsumer = (*Consumer)(nil)
var _ translator.TagsConsumer = (*Consumer)(nil)

// Consumer is the metrics Consumer.
type Consumer struct {
	ms        []datadogV2.MetricSeries
	sl        sketches.SketchSeriesList
	seenHosts map[string]struct{}
	seenTags  map[string]struct{}
}

// NewConsumer creates a new Datadog consumer.
func NewConsumer() *Consumer {
	return &Consumer{
		seenHosts: make(map[string]struct{}),
		seenTags:  make(map[string]struct{}),
	}
}

// toDataType maps translator datatypes to DatadogV2's datatypes.
func (c *Consumer) toDataType(dt translator.MetricDataType) (out datadogV2.MetricIntakeType) {
	out = datadogV2.METRICINTAKETYPE_UNSPECIFIED

	switch dt {
	case translator.Count:
		out = datadogV2.METRICINTAKETYPE_COUNT
	case translator.Gauge:
		out = datadogV2.METRICINTAKETYPE_GAUGE
	}

	return
}

// runningMetrics gets the running metrics for the exporter.
func (c *Consumer) runningMetrics(timestamp uint64, buildInfo component.BuildInfo) (series []datadogV2.MetricSeries) {
	for host := range c.seenHosts {
		// Report the host as running
		runningMetric := DefaultMetrics("metrics", host, timestamp, buildInfo)
		series = append(series, runningMetric...)
	}

	for tag := range c.seenTags {
		runningMetrics := DefaultMetrics("metrics", "", timestamp, buildInfo)
		for i := range runningMetrics {
			runningMetrics[i].Tags = append(runningMetrics[i].Tags, tag)
		}
		series = append(series, runningMetrics...)
	}

	return
}

// All gets all metrics (consumed metrics and running metrics).
func (c *Consumer) All(timestamp uint64, buildInfo component.BuildInfo, tags []string) ([]datadogV2.MetricSeries, sketches.SketchSeriesList) {
	series := c.ms
	series = append(series, c.runningMetrics(timestamp, buildInfo)...)
	if len(tags) == 0 {
		return series, c.sl
	}
	for i := 0; i < len(series); i++ {
		series[i].Tags = append(series[i].Tags, tags...)
	}
	for i := 0; i < len(c.sl); i++ {
		c.sl[i].Tags = append(c.sl[i].Tags, tags...)
	}
	return series, c.sl
}

// ConsumeTimeSeries implements the translator.Consumer interface.
func (c *Consumer) ConsumeTimeSeries(
	_ context.Context,
	dims *translator.Dimensions,
	typ translator.MetricDataType,
	timestamp uint64,
	value float64,
) {
	dt := c.toDataType(typ)
	met := NewMetric(dims.Name(), dt, timestamp, value, dims.Tags())
	met.SetResources([]datadogV2.MetricResource{
		{
			Name: datadog.PtrString(dims.Host()),
			Type: datadog.PtrString("host"),
		},
	})
	c.ms = append(c.ms, met)
}

// ConsumeSketch implements the translator.Consumer interface.
func (c *Consumer) ConsumeSketch(
	_ context.Context,
	dims *translator.Dimensions,
	timestamp uint64,
	sketch *quantile.Sketch,
) {
	c.sl = append(c.sl, sketches.SketchSeries{
		Name:     dims.Name(),
		Tags:     dims.Tags(),
		Host:     dims.Host(),
		Interval: 1,
		Points: []sketches.SketchPoint{{
			Ts:     int64(timestamp / 1e9),
			Sketch: sketch,
		}},
	})
}

// ConsumeHost implements the translator.HostConsumer interface.
func (c *Consumer) ConsumeHost(host string) {
	c.seenHosts[host] = struct{}{}
}

// ConsumeTag implements the translator.TagsConsumer interface.
func (c *Consumer) ConsumeTag(tag string) {
	c.seenTags[tag] = struct{}{}
}

var _ translator.Consumer = (*ZorkianConsumer)(nil)
var _ translator.HostConsumer = (*ZorkianConsumer)(nil)
var _ translator.TagsConsumer = (*ZorkianConsumer)(nil)

// ZorkianConsumer is the metrics Consumer using Zorkian APIs.
// Deprecated: Zorkian is deprecated, use Consumer instead.
type ZorkianConsumer struct {
	ms        []zorkian.Metric
	sl        sketches.SketchSeriesList
	seenHosts map[string]struct{}
	seenTags  map[string]struct{}
}

// NewZorkianConsumer creates a new Zorkian Datadog consumer.
// Deprecated: Zorkian is deprecated, use NewConsumer instead.
func NewZorkianConsumer() *ZorkianConsumer {
	return &ZorkianConsumer{
		seenHosts: make(map[string]struct{}),
		seenTags:  make(map[string]struct{}),
	}
}

// toDataType maps translator datatypes to Zorkian's datatypes.
func (c *ZorkianConsumer) toDataType(dt translator.MetricDataType) (out MetricType) {
	out = MetricType("unknown")

	switch dt {
	case translator.Count:
		out = Count
	case translator.Gauge:
		out = Gauge
	}

	return
}

// runningMetrics gets the running metrics for the exporter.
func (c *ZorkianConsumer) runningMetrics(timestamp uint64, buildInfo component.BuildInfo) (series []zorkian.Metric) {
	for host := range c.seenHosts {
		// Report the host as running
		runningMetric := DefaultZorkianMetrics("metrics", host, timestamp, buildInfo)
		series = append(series, runningMetric...)
	}

	for tag := range c.seenTags {
		runningMetrics := DefaultZorkianMetrics("metrics", "", timestamp, buildInfo)
		for i := range runningMetrics {
			runningMetrics[i].Tags = append(runningMetrics[i].Tags, tag)
		}
		series = append(series, runningMetrics...)
	}

	return
}

// All gets all metrics (consumed metrics and running metrics).
func (c *ZorkianConsumer) All(timestamp uint64, buildInfo component.BuildInfo, tags []string) ([]zorkian.Metric, sketches.SketchSeriesList) {
	series := c.ms
	series = append(series, c.runningMetrics(timestamp, buildInfo)...)
	if len(tags) == 0 {
		return series, c.sl
	}
	for i := 0; i < len(series); i++ {
		series[i].Tags = append(series[i].Tags, tags...)
	}
	for i := 0; i < len(c.sl); i++ {
		c.sl[i].Tags = append(c.sl[i].Tags, tags...)
	}
	return series, c.sl
}

// ConsumeTimeSeries implements the translator.Consumer interface.
func (c *ZorkianConsumer) ConsumeTimeSeries(
	_ context.Context,
	dims *translator.Dimensions,
	typ translator.MetricDataType,
	timestamp uint64,
	value float64,
) {
	dt := c.toDataType(typ)
	met := NewZorkianMetric(dims.Name(), dt, timestamp, value, dims.Tags())
	met.SetHost(dims.Host())
	c.ms = append(c.ms, met)
}

// ConsumeSketch implements the translator.Consumer interface.
func (c *ZorkianConsumer) ConsumeSketch(
	_ context.Context,
	dims *translator.Dimensions,
	timestamp uint64,
	sketch *quantile.Sketch,
) {
	c.sl = append(c.sl, sketches.SketchSeries{
		Name:     dims.Name(),
		Tags:     dims.Tags(),
		Host:     dims.Host(),
		Interval: 1,
		Points: []sketches.SketchPoint{{
			Ts:     int64(timestamp / 1e9),
			Sketch: sketch,
		}},
	})
}

// ConsumeHost implements the translator.HostConsumer interface.
func (c *ZorkianConsumer) ConsumeHost(host string) {
	c.seenHosts[host] = struct{}{}
}

// ConsumeTag implements the translator.TagsConsumer interface.
func (c *ZorkianConsumer) ConsumeTag(tag string) {
	c.seenTags[tag] = struct{}{}
}
