// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package pmetricutiltest // import "github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector/internal/pmetricutiltest"

import "go.opentelemetry.io/collector/pdata/pmetric"

// NewMetrics returns a pmetric.Metrics with a uniform structure where resources, scopes, metrics,
// and datapoints are identical across all instances, except for one identifying field.
//
// Identifying fields:
// - Resources have an attribute called "resourceName" with a value of "resourceN".
// - Scopes have a name with a value of "scopeN".
// - Metrics have a name with a value of "metricN" and a single time series of data points.
// - DataPoints have an attribute "dpName" with a value of "dpN".
//
// Example: NewMetrics("AB", "XYZ", "MN", "1234") returns:
//
//	resourceA, resourceB
//	    each with scopeX, scopeY, scopeZ
//	        each with metricM, metricN
//	            each with dp1, dp2, dp3, dp4
//
// Each byte in the input string is a unique ID for the corresponding element.
func NewMetrics(resourceIDs, scopeIDs, metricIDs, dataPointIDs string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	for resourceN := 0; resourceN < len(resourceIDs); resourceN++ {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("resourceName", "resource"+string(resourceIDs[resourceN]))
		for scopeN := 0; scopeN < len(scopeIDs); scopeN++ {
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName("scope" + string(scopeIDs[scopeN]))
			for metricN := 0; metricN < len(metricIDs); metricN++ {
				m := sm.Metrics().AppendEmpty()
				m.SetName("metric" + string(metricIDs[metricN]))
				dps := m.SetEmptyGauge()
				for dataPointN := 0; dataPointN < len(dataPointIDs); dataPointN++ {
					dp := dps.DataPoints().AppendEmpty()
					dp.Attributes().PutStr("dpName", "dp"+string(dataPointIDs[dataPointN]))
				}
			}
		}
	}
	return md
}

func NewMetricsFromOpts(resources ...pmetric.ResourceMetrics) pmetric.Metrics {
	md := pmetric.NewMetrics()
	for _, resource := range resources {
		resource.CopyTo(md.ResourceMetrics().AppendEmpty())
	}
	return md
}

func Resource(id string, scopes ...pmetric.ScopeMetrics) pmetric.ResourceMetrics {
	rm := pmetric.NewResourceMetrics()
	rm.Resource().Attributes().PutStr("resourceName", "resource"+id)
	for _, scope := range scopes {
		scope.CopyTo(rm.ScopeMetrics().AppendEmpty())
	}
	return rm
}

func Scope(id string, metrics ...pmetric.Metric) pmetric.ScopeMetrics {
	s := pmetric.NewScopeMetrics()
	s.Scope().SetName("scope" + id)
	for _, metric := range metrics {
		metric.CopyTo(s.Metrics().AppendEmpty())
	}
	return s
}

func Metric(id string, dps ...pmetric.NumberDataPoint) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName("metric" + id)
	g := m.SetEmptyGauge()
	for _, dp := range dps {
		dp.CopyTo(g.DataPoints().AppendEmpty())
	}
	return m
}

func NumberDataPoint(id string) pmetric.NumberDataPoint {
	dp := pmetric.NewNumberDataPoint()
	dp.Attributes().PutStr("dpName", "dp"+id)
	return dp
}