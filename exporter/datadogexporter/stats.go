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

package datadogexporter

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

const (
	statsBucketDuration int64 = 1e10 // 10 seconds
)

// ComputeAPMStats calculates the stats that should be submitted to APM about a given trace
func ComputeAPMStats(tracePayload *pb.TracePayload) *stats.Payload {

	statsRawBuckets := make(map[int64]*stats.RawBucket)
	statsDuration := statsBucketDuration
	var earliestStart int64 = 1e0
	var earliestEnd int64 = 1e0

	for _, trace := range tracePayload.Traces {
		for _, span := range trace.Spans {
			tempStart := span.Start

			tempEnd := span.Start + span.Duration

			if earliestStart == 1e0 {
				earliestStart = tempStart
			} else if tempStart < earliestStart {
				earliestStart = tempStart
			}
			if tempEnd > earliestEnd {
				earliestEnd = tempEnd
			}
		}
	}

	if earliestStart != 1e0 {
		fmt.Println("duration was")
		fmt.Println(statsDuration)
		statsDuration = earliestEnd - earliestStart
		fmt.Println("duration is now")
		fmt.Println(statsDuration)
	}

	for _, trace := range tracePayload.Traces {
		spans := GetAnalyzedSpans(trace.Spans)
		sublayers := stats.ComputeSublayers(trace.Spans)
		for _, span := range spans {

			// Aggregate the span to a bucket by rounding its end timestamp to the closest bucket ts.
			// E.g., for buckets of size 10, a span ends on 36 should be aggregated to the second bucket
			// with bucketTS 30 (36 - 36 % 10). Create a new bucket if needed.
			spanEnd := span.Start + span.Duration
			bucketTS := spanEnd - (spanEnd % statsDuration)
			var statsRawBucket *stats.RawBucket
			if existingBucket, ok := statsRawBuckets[bucketTS]; ok {
				statsRawBucket = existingBucket
			} else {
				statsRawBucket = stats.NewRawBucket(bucketTS, statsDuration)
				statsRawBuckets[bucketTS] = statsRawBucket
			}

			// Use weight 1, as sampling in opentelemetry would occur upstream in a processor.
			// Generally we want to ship 100% of traces to the backend where more accurate tail based sampling can be performed.
			// TopLevel is always "true" since we only compute stats for top-level spans.
			weightedSpan := &stats.WeightedSpan{
				Span:     span,
				Weight:   1,
				TopLevel: true,
			}
			statsRawBucket.HandleSpan(weightedSpan, tracePayload.Env, []string{}, sublayers)

			// fmt.Println("Span")
			// fmt.Println(span)
			// fmt.Println("Raw Bucket")
			// fmt.Println(statsRawBucket.Export())
		}
	}

	// Export statsRawBuckets to statsBuckets
	statsBuckets := make([]stats.Bucket, 0)
	for _, statsRawBucket := range statsRawBuckets {
		statsBuckets = append(statsBuckets, statsRawBucket.Export())
	}

	
	return &stats.Payload{
		HostName: tracePayload.HostName,
		Env:      tracePayload.Env,
		Stats:    statsBuckets,
	}
}
