// Code generated by mdatagen. DO NOT EDIT.

package metadata

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDefaultMetrics(t *testing.T) {
	start := pcommon.Timestamp(1_000_000_000)
	ts := pcommon.Timestamp(1_000_001_000)
	mb := NewMetricsBuilder(DefaultMetricsSettings(), componenttest.NewNopReceiverCreateSettings(), WithStartTime(start))
	enabledMetrics := make(map[string]bool)

	enabledMetrics["httpcheck.duration"] = true
	mb.RecordHttpcheckDurationDataPoint(ts, 1, "attr-val")

	enabledMetrics["httpcheck.error"] = true
	mb.RecordHttpcheckErrorDataPoint(ts, 1, "attr-val", "attr-val")

	enabledMetrics["httpcheck.status"] = true
	mb.RecordHttpcheckStatusDataPoint(ts, 1, "attr-val", 1, "attr-val", "attr-val")

	metrics := mb.Emit()

	assert.Equal(t, 1, metrics.ResourceMetrics().Len())
	sm := metrics.ResourceMetrics().At(0).ScopeMetrics()
	assert.Equal(t, 1, sm.Len())
	ms := sm.At(0).Metrics()
	assert.Equal(t, len(enabledMetrics), ms.Len())
	seenMetrics := make(map[string]bool)
	for i := 0; i < ms.Len(); i++ {
		assert.True(t, enabledMetrics[ms.At(i).Name()])
		seenMetrics[ms.At(i).Name()] = true
	}
	assert.Equal(t, len(enabledMetrics), len(seenMetrics))
}

func TestAllMetrics(t *testing.T) {
	start := pcommon.Timestamp(1_000_000_000)
	ts := pcommon.Timestamp(1_000_001_000)
	metricsSettings := MetricsSettings{
		HttpcheckDuration: MetricSettings{Enabled: true},
		HttpcheckError:    MetricSettings{Enabled: true},
		HttpcheckStatus:   MetricSettings{Enabled: true},
	}
	observedZapCore, observedLogs := observer.New(zap.WarnLevel)
	settings := componenttest.NewNopReceiverCreateSettings()
	settings.Logger = zap.New(observedZapCore)
	mb := NewMetricsBuilder(metricsSettings, settings, WithStartTime(start))

	assert.Equal(t, 0, observedLogs.Len())

	mb.RecordHttpcheckDurationDataPoint(ts, 1, "attr-val")
	mb.RecordHttpcheckErrorDataPoint(ts, 1, "attr-val", "attr-val")
	mb.RecordHttpcheckStatusDataPoint(ts, 1, "attr-val", 1, "attr-val", "attr-val")

	metrics := mb.Emit()

	assert.Equal(t, 1, metrics.ResourceMetrics().Len())
	rm := metrics.ResourceMetrics().At(0)
	attrCount := 0
	assert.Equal(t, attrCount, rm.Resource().Attributes().Len())

	assert.Equal(t, 1, rm.ScopeMetrics().Len())
	ms := rm.ScopeMetrics().At(0).Metrics()
	allMetricsCount := reflect.TypeOf(MetricsSettings{}).NumField()
	assert.Equal(t, allMetricsCount, ms.Len())
	validatedMetrics := make(map[string]struct{})
	for i := 0; i < ms.Len(); i++ {
		switch ms.At(i).Name() {
		case "httpcheck.duration":
			assert.Equal(t, pmetric.MetricTypeGauge, ms.At(i).Type())
			assert.Equal(t, 1, ms.At(i).Gauge().DataPoints().Len())
			assert.Equal(t, "Measures the duration of the HTTP check.", ms.At(i).Description())
			assert.Equal(t, "ms", ms.At(i).Unit())
			dp := ms.At(i).Gauge().DataPoints().At(0)
			assert.Equal(t, start, dp.StartTimestamp())
			assert.Equal(t, ts, dp.Timestamp())
			assert.Equal(t, pmetric.NumberDataPointValueTypeInt, dp.ValueType())
			assert.Equal(t, int64(1), dp.IntValue())
			attrVal, ok := dp.Attributes().Get("http.url")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			validatedMetrics["httpcheck.duration"] = struct{}{}
		case "httpcheck.error":
			assert.Equal(t, pmetric.MetricTypeSum, ms.At(i).Type())
			assert.Equal(t, 1, ms.At(i).Sum().DataPoints().Len())
			assert.Equal(t, "Records errors occurring during HTTP check.", ms.At(i).Description())
			assert.Equal(t, "{error}", ms.At(i).Unit())
			assert.Equal(t, false, ms.At(i).Sum().IsMonotonic())
			assert.Equal(t, pmetric.AggregationTemporalityCumulative, ms.At(i).Sum().AggregationTemporality())
			dp := ms.At(i).Sum().DataPoints().At(0)
			assert.Equal(t, start, dp.StartTimestamp())
			assert.Equal(t, ts, dp.Timestamp())
			assert.Equal(t, pmetric.NumberDataPointValueTypeInt, dp.ValueType())
			assert.Equal(t, int64(1), dp.IntValue())
			attrVal, ok := dp.Attributes().Get("http.url")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			attrVal, ok = dp.Attributes().Get("error.message")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			validatedMetrics["httpcheck.error"] = struct{}{}
		case "httpcheck.status":
			assert.Equal(t, pmetric.MetricTypeSum, ms.At(i).Type())
			assert.Equal(t, 1, ms.At(i).Sum().DataPoints().Len())
			assert.Equal(t, "1 if the check resulted in status_code matching the status_class, otherwise 0.", ms.At(i).Description())
			assert.Equal(t, "1", ms.At(i).Unit())
			assert.Equal(t, false, ms.At(i).Sum().IsMonotonic())
			assert.Equal(t, pmetric.AggregationTemporalityCumulative, ms.At(i).Sum().AggregationTemporality())
			dp := ms.At(i).Sum().DataPoints().At(0)
			assert.Equal(t, start, dp.StartTimestamp())
			assert.Equal(t, ts, dp.Timestamp())
			assert.Equal(t, pmetric.NumberDataPointValueTypeInt, dp.ValueType())
			assert.Equal(t, int64(1), dp.IntValue())
			attrVal, ok := dp.Attributes().Get("http.url")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			attrVal, ok = dp.Attributes().Get("http.status_code")
			assert.True(t, ok)
			assert.EqualValues(t, 1, attrVal.Int())
			attrVal, ok = dp.Attributes().Get("http.method")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			attrVal, ok = dp.Attributes().Get("http.status_class")
			assert.True(t, ok)
			assert.EqualValues(t, "attr-val", attrVal.Str())
			validatedMetrics["httpcheck.status"] = struct{}{}
		}
	}
	assert.Equal(t, allMetricsCount, len(validatedMetrics))
}

func TestNoMetrics(t *testing.T) {
	start := pcommon.Timestamp(1_000_000_000)
	ts := pcommon.Timestamp(1_000_001_000)
	metricsSettings := MetricsSettings{
		HttpcheckDuration: MetricSettings{Enabled: false},
		HttpcheckError:    MetricSettings{Enabled: false},
		HttpcheckStatus:   MetricSettings{Enabled: false},
	}
	observedZapCore, observedLogs := observer.New(zap.WarnLevel)
	settings := componenttest.NewNopReceiverCreateSettings()
	settings.Logger = zap.New(observedZapCore)
	mb := NewMetricsBuilder(metricsSettings, settings, WithStartTime(start))

	assert.Equal(t, 0, observedLogs.Len())
	mb.RecordHttpcheckDurationDataPoint(ts, 1, "attr-val")
	mb.RecordHttpcheckErrorDataPoint(ts, 1, "attr-val", "attr-val")
	mb.RecordHttpcheckStatusDataPoint(ts, 1, "attr-val", 1, "attr-val", "attr-val")

	metrics := mb.Emit()

	assert.Equal(t, 0, metrics.ResourceMetrics().Len())
}
