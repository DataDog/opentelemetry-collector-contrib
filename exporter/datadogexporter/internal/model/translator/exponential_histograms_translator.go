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
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/quantile"
	"go.opentelemetry.io/collector/model/pdata"
	"go.uber.org/zap"
)

func (t *Translator) exponentialHistogramToSketch(
	p pdata.ExponentialHistogramDataPoint,
	delta bool,
) (*quantile.Sketch, error) {
	if !delta {
		return nil, fmt.Errorf("cumulative exponential histograms are not supported")
	}

	as := &quantile.Agent{}

	gamma := math.Pow(2, math.Pow(2, -float64(p.Scale())))

	for j, count := range p.Positive().BucketCounts() {
		// Find the real index of the bucket by adding the offset
		index := j + int(p.Positive().Offset())

		lowerBound := math.Pow(gamma, float64(index))
		upperBound := math.Pow(gamma, float64(index + 1))
		as.InsertInterpolate(lowerBound, upperBound, uint(count))
	}

	for j, count := range p.Negative().BucketCounts() {
		// Find the real index of the bucket by adding the offset
		index := j + int(p.Negative().Offset())

		upperBound := -math.Pow(gamma, float64(index))
		lowerBound := -math.Pow(gamma, float64(index + 1))
		as.InsertInterpolate(lowerBound, upperBound, uint(count))
	}

	sketch := as.Finish()
	return sketch, nil
}


// mapExponentialHistogramMetrics maps exponential histogram metrics slices to Datadog metrics
//
// An ExponentialHistogram metric has:
// - The count of values in the population
// - The sum of values in the population
// - A scale, from which the base of the exponential histogram is computed
// - Two bucket stores, each with:
//     - an offset
//     - a list of bucket counts
// - A count of zero values in the population
func (t *Translator) mapExponentialHistogramMetrics(
	ctx context.Context,
	consumer Consumer,
	dims *Dimensions,
	slice pdata.ExponentialHistogramDataPointSlice,
	delta bool,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		startTs := uint64(p.StartTimestamp())
		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		histInfo := histogramInfo{ok: true}

		countDims := pointDims.WithSuffix("count")
		if delta {
			histInfo.count = p.Count()
		} else if dx, ok := t.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok {
			histInfo.count = uint64(dx)
		} else { // not ok
			histInfo.ok = false
		}

		sumDims := pointDims.WithSuffix("sum")
		if !t.isSkippable(sumDims.name, p.Sum()) {
			if delta {
				histInfo.sum = p.Sum()
			} else if dx, ok := t.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
				histInfo.sum = dx
			} else { // not ok
				histInfo.ok = false
			}
		} else { // skippable
			histInfo.ok = false
		}

		if t.cfg.SendCountSum && histInfo.ok {
			// We only send the sum and count if both values were ok.
			consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, float64(histInfo.count))
			consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, histInfo.sum)
		}

		agentSketch, err := t.exponentialHistogramToSketch(p, delta)
		if err != nil {
			t.logger.Debug("Failed to convert ExponentialHistogram into DDSketch",
				zap.String("metric name", dims.name),
				zap.Error(err),
			)
			continue
		}

		if histInfo.ok {
			// override approximate sum, count and average in sketch with exact values if available.
			agentSketch.Basic.Cnt = int64(histInfo.count)
			agentSketch.Basic.Sum = histInfo.sum
			agentSketch.Basic.Avg = agentSketch.Basic.Sum / float64(agentSketch.Basic.Cnt)
		}

		consumer.ConsumeSketch(ctx, pointDims, ts, agentSketch)
	}
}
