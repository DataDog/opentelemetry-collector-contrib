// Code generated by mdatagen. DO NOT EDIT.

package metadata

import (
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	conventions "go.opentelemetry.io/collector/semconv/v1.18.0"
)

type metricK8sStatefulsetCurrentPods struct {
	data     pmetric.Metric // data buffer for generated metric.
	config   MetricConfig   // metric config provided by user.
	capacity int            // max observed number of data points added to the metric.
}

// init fills k8s.statefulset.current_pods metric with initial data.
func (m *metricK8sStatefulsetCurrentPods) init() {
	m.data.SetName("k8s.statefulset.current_pods")
	m.data.SetDescription("The number of pods created by the StatefulSet controller from the StatefulSet version")
	m.data.SetUnit("1")
	m.data.SetEmptyGauge()
}

func (m *metricK8sStatefulsetCurrentPods) recordDataPoint(start pcommon.Timestamp, ts pcommon.Timestamp, val int64) {
	if !m.config.Enabled {
		return
	}
	dp := m.data.Gauge().DataPoints().AppendEmpty()
	dp.SetStartTimestamp(start)
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
}

// updateCapacity saves max length of data point slices that will be used for the slice capacity.
func (m *metricK8sStatefulsetCurrentPods) updateCapacity() {
	if m.data.Gauge().DataPoints().Len() > m.capacity {
		m.capacity = m.data.Gauge().DataPoints().Len()
	}
}

// emit appends recorded metric data to a metrics slice and prepares it for recording another set of data points.
func (m *metricK8sStatefulsetCurrentPods) emit(metrics pmetric.MetricSlice) {
	if m.config.Enabled && m.data.Gauge().DataPoints().Len() > 0 {
		m.updateCapacity()
		m.data.MoveTo(metrics.AppendEmpty())
		m.init()
	}
}

func newMetricK8sStatefulsetCurrentPods(cfg MetricConfig) metricK8sStatefulsetCurrentPods {
	m := metricK8sStatefulsetCurrentPods{config: cfg}
	if cfg.Enabled {
		m.data = pmetric.NewMetric()
		m.init()
	}
	return m
}

type metricK8sStatefulsetDesiredPods struct {
	data     pmetric.Metric // data buffer for generated metric.
	config   MetricConfig   // metric config provided by user.
	capacity int            // max observed number of data points added to the metric.
}

// init fills k8s.statefulset.desired_pods metric with initial data.
func (m *metricK8sStatefulsetDesiredPods) init() {
	m.data.SetName("k8s.statefulset.desired_pods")
	m.data.SetDescription("Number of desired pods in the stateful set (the `spec.replicas` field)")
	m.data.SetUnit("1")
	m.data.SetEmptyGauge()
}

func (m *metricK8sStatefulsetDesiredPods) recordDataPoint(start pcommon.Timestamp, ts pcommon.Timestamp, val int64) {
	if !m.config.Enabled {
		return
	}
	dp := m.data.Gauge().DataPoints().AppendEmpty()
	dp.SetStartTimestamp(start)
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
}

// updateCapacity saves max length of data point slices that will be used for the slice capacity.
func (m *metricK8sStatefulsetDesiredPods) updateCapacity() {
	if m.data.Gauge().DataPoints().Len() > m.capacity {
		m.capacity = m.data.Gauge().DataPoints().Len()
	}
}

// emit appends recorded metric data to a metrics slice and prepares it for recording another set of data points.
func (m *metricK8sStatefulsetDesiredPods) emit(metrics pmetric.MetricSlice) {
	if m.config.Enabled && m.data.Gauge().DataPoints().Len() > 0 {
		m.updateCapacity()
		m.data.MoveTo(metrics.AppendEmpty())
		m.init()
	}
}

func newMetricK8sStatefulsetDesiredPods(cfg MetricConfig) metricK8sStatefulsetDesiredPods {
	m := metricK8sStatefulsetDesiredPods{config: cfg}
	if cfg.Enabled {
		m.data = pmetric.NewMetric()
		m.init()
	}
	return m
}

type metricK8sStatefulsetReadyPods struct {
	data     pmetric.Metric // data buffer for generated metric.
	config   MetricConfig   // metric config provided by user.
	capacity int            // max observed number of data points added to the metric.
}

// init fills k8s.statefulset.ready_pods metric with initial data.
func (m *metricK8sStatefulsetReadyPods) init() {
	m.data.SetName("k8s.statefulset.ready_pods")
	m.data.SetDescription("Number of pods created by the stateful set that have the `Ready` condition")
	m.data.SetUnit("1")
	m.data.SetEmptyGauge()
}

func (m *metricK8sStatefulsetReadyPods) recordDataPoint(start pcommon.Timestamp, ts pcommon.Timestamp, val int64) {
	if !m.config.Enabled {
		return
	}
	dp := m.data.Gauge().DataPoints().AppendEmpty()
	dp.SetStartTimestamp(start)
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
}

// updateCapacity saves max length of data point slices that will be used for the slice capacity.
func (m *metricK8sStatefulsetReadyPods) updateCapacity() {
	if m.data.Gauge().DataPoints().Len() > m.capacity {
		m.capacity = m.data.Gauge().DataPoints().Len()
	}
}

// emit appends recorded metric data to a metrics slice and prepares it for recording another set of data points.
func (m *metricK8sStatefulsetReadyPods) emit(metrics pmetric.MetricSlice) {
	if m.config.Enabled && m.data.Gauge().DataPoints().Len() > 0 {
		m.updateCapacity()
		m.data.MoveTo(metrics.AppendEmpty())
		m.init()
	}
}

func newMetricK8sStatefulsetReadyPods(cfg MetricConfig) metricK8sStatefulsetReadyPods {
	m := metricK8sStatefulsetReadyPods{config: cfg}
	if cfg.Enabled {
		m.data = pmetric.NewMetric()
		m.init()
	}
	return m
}

type metricK8sStatefulsetUpdatedPods struct {
	data     pmetric.Metric // data buffer for generated metric.
	config   MetricConfig   // metric config provided by user.
	capacity int            // max observed number of data points added to the metric.
}

// init fills k8s.statefulset.updated_pods metric with initial data.
func (m *metricK8sStatefulsetUpdatedPods) init() {
	m.data.SetName("k8s.statefulset.updated_pods")
	m.data.SetDescription("Number of pods created by the StatefulSet controller from the StatefulSet version")
	m.data.SetUnit("1")
	m.data.SetEmptyGauge()
}

func (m *metricK8sStatefulsetUpdatedPods) recordDataPoint(start pcommon.Timestamp, ts pcommon.Timestamp, val int64) {
	if !m.config.Enabled {
		return
	}
	dp := m.data.Gauge().DataPoints().AppendEmpty()
	dp.SetStartTimestamp(start)
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
}

// updateCapacity saves max length of data point slices that will be used for the slice capacity.
func (m *metricK8sStatefulsetUpdatedPods) updateCapacity() {
	if m.data.Gauge().DataPoints().Len() > m.capacity {
		m.capacity = m.data.Gauge().DataPoints().Len()
	}
}

// emit appends recorded metric data to a metrics slice and prepares it for recording another set of data points.
func (m *metricK8sStatefulsetUpdatedPods) emit(metrics pmetric.MetricSlice) {
	if m.config.Enabled && m.data.Gauge().DataPoints().Len() > 0 {
		m.updateCapacity()
		m.data.MoveTo(metrics.AppendEmpty())
		m.init()
	}
}

func newMetricK8sStatefulsetUpdatedPods(cfg MetricConfig) metricK8sStatefulsetUpdatedPods {
	m := metricK8sStatefulsetUpdatedPods{config: cfg}
	if cfg.Enabled {
		m.data = pmetric.NewMetric()
		m.init()
	}
	return m
}

// MetricsBuilder provides an interface for scrapers to report metrics while taking care of all the transformations
// required to produce metric representation defined in metadata and user config.
type MetricsBuilder struct {
	startTime                       pcommon.Timestamp   // start time that will be applied to all recorded data points.
	metricsCapacity                 int                 // maximum observed number of metrics per resource.
	metricsBuffer                   pmetric.Metrics     // accumulates metrics data before emitting.
	buildInfo                       component.BuildInfo // contains version information
	metricK8sStatefulsetCurrentPods metricK8sStatefulsetCurrentPods
	metricK8sStatefulsetDesiredPods metricK8sStatefulsetDesiredPods
	metricK8sStatefulsetReadyPods   metricK8sStatefulsetReadyPods
	metricK8sStatefulsetUpdatedPods metricK8sStatefulsetUpdatedPods
}

// metricBuilderOption applies changes to default metrics builder.
type metricBuilderOption func(*MetricsBuilder)

// WithStartTime sets startTime on the metrics builder.
func WithStartTime(startTime pcommon.Timestamp) metricBuilderOption {
	return func(mb *MetricsBuilder) {
		mb.startTime = startTime
	}
}

func NewMetricsBuilder(mbc MetricsBuilderConfig, settings receiver.CreateSettings, options ...metricBuilderOption) *MetricsBuilder {
	mb := &MetricsBuilder{
		startTime:                       pcommon.NewTimestampFromTime(time.Now()),
		metricsBuffer:                   pmetric.NewMetrics(),
		buildInfo:                       settings.BuildInfo,
		metricK8sStatefulsetCurrentPods: newMetricK8sStatefulsetCurrentPods(mbc.Metrics.K8sStatefulsetCurrentPods),
		metricK8sStatefulsetDesiredPods: newMetricK8sStatefulsetDesiredPods(mbc.Metrics.K8sStatefulsetDesiredPods),
		metricK8sStatefulsetReadyPods:   newMetricK8sStatefulsetReadyPods(mbc.Metrics.K8sStatefulsetReadyPods),
		metricK8sStatefulsetUpdatedPods: newMetricK8sStatefulsetUpdatedPods(mbc.Metrics.K8sStatefulsetUpdatedPods),
	}
	for _, op := range options {
		op(mb)
	}
	return mb
}

// updateCapacity updates max length of metrics and resource attributes that will be used for the slice capacity.
func (mb *MetricsBuilder) updateCapacity(rm pmetric.ResourceMetrics) {
	if mb.metricsCapacity < rm.ScopeMetrics().At(0).Metrics().Len() {
		mb.metricsCapacity = rm.ScopeMetrics().At(0).Metrics().Len()
	}
}

// ResourceMetricsOption applies changes to provided resource metrics.
type ResourceMetricsOption func(pmetric.ResourceMetrics)

// WithResource sets the provided resource on the emitted ResourceMetrics.
// It's recommended to use ResourceBuilder to create the resource.
func WithResource(res pcommon.Resource) ResourceMetricsOption {
	return func(rm pmetric.ResourceMetrics) {
		res.CopyTo(rm.Resource())
	}
}

// WithStartTimeOverride overrides start time for all the resource metrics data points.
// This option should be only used if different start time has to be set on metrics coming from different resources.
func WithStartTimeOverride(start pcommon.Timestamp) ResourceMetricsOption {
	return func(rm pmetric.ResourceMetrics) {
		var dps pmetric.NumberDataPointSlice
		metrics := rm.ScopeMetrics().At(0).Metrics()
		for i := 0; i < metrics.Len(); i++ {
			switch metrics.At(i).Type() {
			case pmetric.MetricTypeGauge:
				dps = metrics.At(i).Gauge().DataPoints()
			case pmetric.MetricTypeSum:
				dps = metrics.At(i).Sum().DataPoints()
			}
			for j := 0; j < dps.Len(); j++ {
				dps.At(j).SetStartTimestamp(start)
			}
		}
	}
}

// EmitForResource saves all the generated metrics under a new resource and updates the internal state to be ready for
// recording another set of data points as part of another resource. This function can be helpful when one scraper
// needs to emit metrics from several resources. Otherwise calling this function is not required,
// just `Emit` function can be called instead.
// Resource attributes should be provided as ResourceMetricsOption arguments.
func (mb *MetricsBuilder) EmitForResource(rmo ...ResourceMetricsOption) {
	rm := pmetric.NewResourceMetrics()
	rm.SetSchemaUrl(conventions.SchemaURL)
	ils := rm.ScopeMetrics().AppendEmpty()
	ils.Scope().SetName("otelcol/k8sclusterreceiver")
	ils.Scope().SetVersion(mb.buildInfo.Version)
	ils.Metrics().EnsureCapacity(mb.metricsCapacity)
	mb.metricK8sStatefulsetCurrentPods.emit(ils.Metrics())
	mb.metricK8sStatefulsetDesiredPods.emit(ils.Metrics())
	mb.metricK8sStatefulsetReadyPods.emit(ils.Metrics())
	mb.metricK8sStatefulsetUpdatedPods.emit(ils.Metrics())

	for _, op := range rmo {
		op(rm)
	}
	if ils.Metrics().Len() > 0 {
		mb.updateCapacity(rm)
		rm.MoveTo(mb.metricsBuffer.ResourceMetrics().AppendEmpty())
	}
}

// Emit returns all the metrics accumulated by the metrics builder and updates the internal state to be ready for
// recording another set of metrics. This function will be responsible for applying all the transformations required to
// produce metric representation defined in metadata and user config, e.g. delta or cumulative.
func (mb *MetricsBuilder) Emit(rmo ...ResourceMetricsOption) pmetric.Metrics {
	mb.EmitForResource(rmo...)
	metrics := mb.metricsBuffer
	mb.metricsBuffer = pmetric.NewMetrics()
	return metrics
}

// RecordK8sStatefulsetCurrentPodsDataPoint adds a data point to k8s.statefulset.current_pods metric.
func (mb *MetricsBuilder) RecordK8sStatefulsetCurrentPodsDataPoint(ts pcommon.Timestamp, val int64) {
	mb.metricK8sStatefulsetCurrentPods.recordDataPoint(mb.startTime, ts, val)
}

// RecordK8sStatefulsetDesiredPodsDataPoint adds a data point to k8s.statefulset.desired_pods metric.
func (mb *MetricsBuilder) RecordK8sStatefulsetDesiredPodsDataPoint(ts pcommon.Timestamp, val int64) {
	mb.metricK8sStatefulsetDesiredPods.recordDataPoint(mb.startTime, ts, val)
}

// RecordK8sStatefulsetReadyPodsDataPoint adds a data point to k8s.statefulset.ready_pods metric.
func (mb *MetricsBuilder) RecordK8sStatefulsetReadyPodsDataPoint(ts pcommon.Timestamp, val int64) {
	mb.metricK8sStatefulsetReadyPods.recordDataPoint(mb.startTime, ts, val)
}

// RecordK8sStatefulsetUpdatedPodsDataPoint adds a data point to k8s.statefulset.updated_pods metric.
func (mb *MetricsBuilder) RecordK8sStatefulsetUpdatedPodsDataPoint(ts pcommon.Timestamp, val int64) {
	mb.metricK8sStatefulsetUpdatedPods.recordDataPoint(mb.startTime, ts, val)
}

// Reset resets metrics builder to its initial state. It should be used when external metrics source is restarted,
// and metrics builder should update its startTime and reset it's internal state accordingly.
func (mb *MetricsBuilder) Reset(options ...metricBuilderOption) {
	mb.startTime = pcommon.NewTimestampFromTime(time.Now())
	for _, op := range options {
		op(mb)
	}
}
