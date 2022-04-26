// Copyright  The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestInt64MetricValueMetadata(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, IntValueType)

	assert.Equal(t, metricName, metadata.Name())
	assert.Equal(t, metricColumnName, metadata.ColumnName())
	assert.Equal(t, metricDataType, metadata.DataType())
	assert.Equal(t, metricUnit, metadata.Unit())
	assert.Equal(t, IntValueType, metadata.ValueType())

	var expectedType *int64

	assert.IsType(t, expectedType, metadata.ValueHolder())
}

func TestFloat64MetricValueMetadata(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, FloatValueType)

	assert.Equal(t, metricName, metadata.Name())
	assert.Equal(t, metricColumnName, metadata.ColumnName())
	assert.Equal(t, metricDataType, metadata.DataType())
	assert.Equal(t, metricUnit, metadata.Unit())
	assert.Equal(t, FloatValueType, metadata.ValueType())

	var expectedType *float64

	assert.IsType(t, expectedType, metadata.ValueHolder())
}

func TestUnknownMetricValueMetadata(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, err := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, UnknownValueType)

	require.Error(t, err)
	require.Nil(t, metadata)
}

func TestInt64MetricValue(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, IntValueType)
	metricValue := int64MetricValue{
		metadata: metadata,
		value:    int64Value,
	}

	assert.Equal(t, int64Value, metricValue.Value())
	assert.Equal(t, IntValueType, metadata.ValueType())

	dataPoint := pmetric.NewNumberDataPoint()

	metricValue.SetValueTo(dataPoint)

	assert.Equal(t, int64Value, dataPoint.IntVal())
}

func TestFloat64MetricValue(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, FloatValueType)
	metricValue := float64MetricValue{
		metadata: metadata,
		value:    float64Value,
	}

	assert.Equal(t, float64Value, metricValue.Value())
	assert.Equal(t, FloatValueType, metadata.ValueType())

	dataPoint := pmetric.NewNumberDataPoint()

	metricValue.SetValueTo(dataPoint)

	assert.Equal(t, float64Value, dataPoint.DoubleVal())
}

func TestNewInt64MetricValue(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, IntValueType)
	value := int64Value
	valueHolder := &value

	metricValue := newInt64MetricValue(metadata, valueHolder)

	assert.Equal(t, int64Value, metricValue.Value())
	assert.Equal(t, IntValueType, metadata.ValueType())
}

func TestNewFloat64MetricValue(t *testing.T) {
	metricDataType := metricValueDataType{dataType: metricDataType}
	metadata, _ := NewMetricValueMetadata(metricName, metricColumnName, metricDataType, metricUnit, FloatValueType)
	value := float64Value
	valueHolder := &value

	metricValue := newFloat64MetricValue(metadata, valueHolder)

	assert.Equal(t, float64Value, metricValue.Value())
	assert.Equal(t, FloatValueType, metadata.ValueType())
}
