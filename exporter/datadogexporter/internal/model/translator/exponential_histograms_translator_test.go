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

package translator // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/model/translator"

import (
	"context"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/quantile/summary"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/model/pdata"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/model/attributes"
)

const (
	acceptableFloatError = 1e-12
)

func TestExponentialHistogramToDDSketch(t *testing.T) {
	ts := pdata.NewTimestampFromTime(time.Now())
	point := pdata.NewExponentialHistogramDataPoint()
	point.SetScale(6)

	point.SetCount(30)
	point.SetZeroCount(10)
	point.SetSum(math.Pi)

	point.Negative().SetOffset(2)
	point.Negative().SetBucketCounts([]uint64{3, 2, 5})

	point.Positive().SetOffset(3)
	point.Positive().SetBucketCounts([]uint64{1, 1, 1, 2, 2, 3})

	point.SetTimestamp(ts)

	tr := newTranslator(t, zap.NewNop())

	sketch, err := tr.exponentialHistogramToDDSketch(point, true)
	assert.NoError(t, err)

	sketch.GetPositiveValueStore().ForEach(func(index int, count float64) bool {
		expectedCount := float64(point.Positive().BucketCounts()[index-int(point.Positive().Offset())])
		assert.Equal(t, expectedCount, count)
		return false
	})

	sketch.GetNegativeValueStore().ForEach(func(index int, count float64) bool {
		expectedCount := float64(point.Negative().BucketCounts()[index-int(point.Negative().Offset())])
		assert.Equal(t, expectedCount, count)
		return false
	})

	assert.Equal(t, float64(point.Count()), sketch.GetCount())
	assert.Equal(t, float64(point.ZeroCount()), sketch.GetCount()-sketch.GetPositiveValueStore().TotalCount()-sketch.GetNegativeValueStore().TotalCount())

	gamma := math.Pow(2, math.Pow(2, float64(-point.Scale())))
	accuracy := (gamma - 1) / (gamma + 1)
	assert.InDelta(t, accuracy, sketch.RelativeAccuracy(), acceptableFloatError)
}

func createExponentialHistogramMetrics(n int, t int, additionalResourceAttributes map[string]string, additionalDatapointAttributes map[string]string) pdata.Metrics {
	md := pdata.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	resourceAttrs := rm.Resource().Attributes()
	resourceAttrs.InsertString(attributes.AttributeDatadogHostname, testHostname)
	for attr, val := range additionalResourceAttributes {
		resourceAttrs.InsertString(attr, val)
	}

	ilms := rm.InstrumentationLibraryMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()

	for i := 0; i < n; i++ {
		met := metricsArray.AppendEmpty()
		met.SetName("expHist.test")
		met.SetDataType(pdata.MetricDataTypeExponentialHistogram)
		met.ExponentialHistogram().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
		points := met.ExponentialHistogram().DataPoints()
		point := points.AppendEmpty()

		datapointAttrs := point.Attributes()
		for attr, val := range additionalDatapointAttributes {
			datapointAttrs.InsertString(attr, val)
		}

		point.SetScale(6)

		point.SetCount(30)
		point.SetZeroCount(10)
		point.SetSum(math.Pi)

		buckets := make([]uint64, t)
		for i := 0; i < t; i++ {
			buckets[i] = 10
		}

		point.Negative().SetOffset(2)
		point.Negative().SetBucketCounts(buckets)

		point.Positive().SetOffset(3)
		point.Positive().SetBucketCounts(buckets)

		point.SetTimestamp(seconds(0))
	}

	return md
}

func TestMapDeltaExponentialHistogramMetrics(t *testing.T) {
	metrics := createExponentialHistogramMetrics(1, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	point := metrics.ResourceMetrics().At(0).InstrumentationLibraryMetrics().At(0).Metrics().At(0).ExponentialHistogram().DataPoints().At(0)
	// gamma = 2^(2^-scale)
	gamma := math.Pow(2, math.Pow(2, -float64(point.Scale())))

	counts := []metric{
		newCountWithHostname("expHist.test.count", 30, uint64(seconds(0)), []string{"attribute_tag:attribute_value"}),
		newCountWithHostname("expHist.test.sum", math.Pi, uint64(seconds(0)), []string{"attribute_tag:attribute_value"}),
	}

	sketches := []sketch{
		newSketchWithHostname("expHist.test", summary.Summary{
			// Expected min: lower bound of the highest negative bucket
			Min: -math.Pow(gamma, float64(int(point.Negative().Offset())+len(point.Negative().BucketCounts()))),
			// Expected max: upper bound of the highest positive bucket
			Max: math.Pow(gamma, float64(int(point.Positive().Offset())+len(point.Positive().BucketCounts()))),
			Sum: point.Sum(),
			Avg: point.Sum() / float64(point.Count()),
			Cnt: int64(point.Count()),
		}, []string{"attribute_tag:attribute_value"}),
	}

	ctx := context.Background()

	tests := []struct {
		name             string
		sendCountSum     bool
		tags             []string
		expectedMetrics  []metric
		expectedSketches []sketch
	}{
		{
			name:             "Send count & sum metrics",
			sendCountSum:     true,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  counts,
			expectedSketches: sketches,
		},
		{
			name:             "Don't send count & sum metrics",
			sendCountSum:     false,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  []metric{},
			expectedSketches: sketches,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			tr := newTranslator(t, zap.NewNop())
			tr.cfg.SendCountSum = testInstance.sendCountSum
			consumer := &mockFullConsumer{}
			tr.MapMetrics(ctx, metrics, consumer)
			assert.ElementsMatch(t, testInstance.expectedMetrics, consumer.metrics)
			// We don't necessarily have strict equality between expected and actual sketches
			// for ExponentialHistograms, therefore we use testMatchingSketches to compare the
			// sketches with more lenient comparisons.
			testMatchingSketches(t, testInstance.expectedSketches, consumer.sketches)
		})
	}
}

func newBenchmarkTranslator(b *testing.B, logger *zap.Logger, opts ...Option) *Translator {
	options := append([]Option{
		WithFallbackHostnameProvider(testProvider("fallbackHostname")),
		WithHistogramMode(HistogramModeDistributions),
		WithNumberMode(NumberModeCumulativeToDelta),
	}, opts...)

	tr, err := New(
		logger,
		options...,
	)

	require.NoError(b, err)
	return tr
}

func benchmarkMapDeltaExponentialHistogramMetrics(metrics pdata.Metrics, b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		tr := newBenchmarkTranslator(b, zap.NewNop())
		consumer := &mockFullConsumer{}
		tr.MapMetrics(ctx, metrics, consumer)
	}
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_5(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_5(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_5(b *testing.B) {
	metrics := createExponentialHistogramMetrics(100, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_5(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1000, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_5(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10000, 5, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_50(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1, 50, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_50(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10, 50, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_50(b *testing.B) {
	metrics := createExponentialHistogramMetrics(100, 50, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_50(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1000, 50, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_50(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10000, 50, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_500(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1, 500, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_500(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10, 500, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_500(b *testing.B) {
	metrics := createExponentialHistogramMetrics(100, 500, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_500(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1000, 500, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_500(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10000, 500, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_5000(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1, 5000, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_5000(b *testing.B) {
	metrics := createExponentialHistogramMetrics(10, 5000, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_5000(b *testing.B) {
	metrics := createExponentialHistogramMetrics(100, 5000, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_5000(b *testing.B) {
	metrics := createExponentialHistogramMetrics(1000, 5000, map[string]string{}, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapDeltaExponentialHistogramMetrics(metrics, b)
}
