// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sample

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

func TestFloatValue(t *testing.T) {
	v := Float(42.0)
	require.Equal(t, TypeFloat, v.Type())
	require.Equal(t, 42.0, v.F())
	require.Nil(t, v.H())
	require.Nil(t, v.FH())
	require.False(t, v.IsHistogram())
	require.NoError(t, v.Validate())
}

func TestHistogramValue(t *testing.T) {
	h := &histogram.Histogram{Sum: 10, Count: 5}
	v := Histogram(h)
	require.Equal(t, TypeHistogram, v.Type())
	require.Equal(t, h, v.H())
	require.Nil(t, v.FH())
	require.True(t, v.IsHistogram())
}

func TestFloatHistogramValue(t *testing.T) {
	fh := &histogram.FloatHistogram{Sum: 10.5, Count: 3}
	v := FloatHistogram(fh)
	require.Equal(t, TypeFloatHistogram, v.Type())
	require.Equal(t, fh, v.FH())
	require.Nil(t, v.H())
	require.True(t, v.IsHistogram())
}

func TestValueIsStale(t *testing.T) {
	staleFloat := math.Float64frombits(value.StaleNaN)

	require.True(t, Float(staleFloat).IsStale())
	require.False(t, Float(42.0).IsStale())

	require.True(t, Histogram(&histogram.Histogram{Sum: staleFloat}).IsStale())
	require.False(t, Histogram(&histogram.Histogram{Sum: 10}).IsStale())

	require.True(t, FloatHistogram(&histogram.FloatHistogram{Sum: staleFloat}).IsStale())
	require.False(t, FloatHistogram(&histogram.FloatHistogram{Sum: 10.5}).IsStale())
}

func TestValueValidate(t *testing.T) {
	// Float is always valid.
	require.NoError(t, Float(0).Validate())
	require.NoError(t, Float(math.NaN()).Validate())

	// Valid histogram.
	require.NoError(t, Histogram(&histogram.Histogram{}).Validate())
	require.NoError(t, FloatHistogram(&histogram.FloatHistogram{}).Validate())

	// Zero value is not valid.
	require.Error(t, Value{}.Validate())
}

func TestValueSetters(t *testing.T) {
	v := Float(1.0)
	require.Equal(t, TypeFloat, v.Type())

	h := &histogram.Histogram{Sum: 5}
	v.SetH(h)
	require.Equal(t, TypeHistogram, v.Type())
	require.Equal(t, h, v.H())
	require.Equal(t, 0.0, v.F())

	fh := &histogram.FloatHistogram{Sum: 7.5}
	v.SetFH(fh)
	require.Equal(t, TypeFloatHistogram, v.Type())
	require.Equal(t, fh, v.FH())
	require.Nil(t, v.H())

	v.SetF(99.0)
	require.Equal(t, TypeFloat, v.Type())
	require.Equal(t, 99.0, v.F())
	require.Nil(t, v.H())
	require.Nil(t, v.FH())
}

func TestTypeString(t *testing.T) {
	require.Equal(t, "float", TypeFloat.String())
	require.Equal(t, "histogram", TypeHistogram.String())
	require.Equal(t, "float_histogram", TypeFloatHistogram.String())
	require.Contains(t, Type(0).String(), "unknown")
}

func BenchmarkFloatValue(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := Float(float64(i))
		_ = v.F()
		_ = v.Type()
	}
}

func BenchmarkHistogramValue(b *testing.B) {
	h := &histogram.Histogram{Sum: 10, Count: 5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := Histogram(h)
		_ = v.H()
		_ = v.Type()
	}
}

func BenchmarkValueIsStale(b *testing.B) {
	v := Float(42.0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.IsStale()
	}
}
