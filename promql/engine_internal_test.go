// Copyright 2024 The Prometheus Authors
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

package promql

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var output bytes.Buffer
	logger := promslog.New(&promslog.Config{Writer: &output})
	ev := &evaluator{logger: logger}

	expr, _ := parser.ParseExpr("sum(up)")

	var err error

	defer func() {
		require.EqualError(t, err, "unexpected error: runtime error: index out of range [123] with length 0")
		require.Contains(t, output.String(), "sum(up)")
	}()
	defer ev.recover(expr, nil, &err)
	// Cause a runtime panic.
	var a []int
	a[123] = 1 //nolint:govet // This is intended to cause a runtime panic.
}

func TestRecoverEvaluatorError(t *testing.T) {
	ev := &evaluator{logger: promslog.NewNopLogger()}
	var err error

	e := errors.New("custom error")

	defer func() {
		require.EqualError(t, err, e.Error())
	}()
	defer ev.recover(nil, nil, &err)

	panic(e)
}

func TestRecoverEvaluatorErrorWithWarnings(t *testing.T) {
	ev := &evaluator{logger: promslog.NewNopLogger()}
	var err error
	var ws annotations.Annotations

	warnings := annotations.New().Add(errors.New("custom warning"))
	e := errWithWarnings{
		err:      errors.New("custom error"),
		warnings: warnings,
	}

	defer func() {
		require.EqualError(t, err, e.Error())
		require.Equal(t, warnings, ws, "wrong warning message")
	}()
	defer ev.recover(nil, &ws, &err)

	panic(e)
}

func TestVectorElemBinop_Histograms(t *testing.T) {
	testCases := []struct {
		name       string
		op         parser.ItemType
		hlhs, hrhs *histogram.FloatHistogram
		want       *histogram.FloatHistogram
		wantErr    error
		wantInfo   error
	}{
		{
			name: "ADD with unknown counter reset hints",
			op:   parser.ADD,
			hlhs: &histogram.FloatHistogram{},
			hrhs: &histogram.FloatHistogram{},
			want: &histogram.FloatHistogram{},
		},
		{
			name: "ADD with mixed counter reset hints",
			op:   parser.ADD,
			hlhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.CounterReset,
			},
			hrhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.NotCounterReset,
			},
			want:    &histogram.FloatHistogram{},
			wantErr: annotations.HistogramCounterResetCollisionWarning,
		},
		{
			name: "SUB with unknown counter reset hints",
			op:   parser.SUB,
			hlhs: &histogram.FloatHistogram{},
			hrhs: &histogram.FloatHistogram{},
			want: &histogram.FloatHistogram{
				CounterResetHint: histogram.GaugeType,
			},
		},
		{
			name: "SUB with mixed counter reset hints",
			op:   parser.SUB,
			hlhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.CounterReset,
			},
			hrhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.NotCounterReset,
			},
			want:    &histogram.FloatHistogram{},
			wantErr: annotations.HistogramCounterResetCollisionWarning,
		},
		{
			name:     "ADD with mismatched NHCB bounds",
			op:       parser.ADD,
			hlhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{1}},
			hrhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{2}},
			want:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}},
			wantInfo: annotations.MismatchedCustomBucketsHistogramsInfo,
		},
		{
			name:     "SUB with mismatched NHCB bounds",
			op:       parser.SUB,
			hlhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{1}},
			hrhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{2}},
			want:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CounterResetHint: histogram.GaugeType},
			wantInfo: annotations.MismatchedCustomBucketsHistogramsInfo,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, _, info, err := vectorElemBinop(tc.op, 0, 0, tc.hlhs, tc.hrhs, posrange.PositionRange{})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)

			if tc.wantInfo != nil {
				require.ErrorIs(t, info, tc.wantInfo)
			} else {
				require.NoError(t, info)
			}

			require.Equal(t, tc.want, got)
		})
	}
}

func TestHandleInfiniteBuckets(t *testing.T) {
	inf := math.Inf(1)
	negInf := math.Inf(-1)

	testCases := []struct {
		name                           string
		bucket                         *histogram.Bucket[float64]
		le                             float64
		isUpperTrim                    bool
		expectedKept, expectedRemoved  float64
	}{
		// Case 1: Bucket with lower bound (-Inf, upper]
		// TRIM_UPPER (</) - remove values greater than le
		{
			name:            "(-Inf, 10] trim_upper le >= upper: keep entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: negInf, Upper: 10, Count: 5},
			le:              10,
			isUpperTrim:     true,
			expectedKept:    5,
			expectedRemoved: 0,
		},
		{
			name:            "(-Inf, 10] trim_upper le > upper: keep entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: negInf, Upper: 10, Count: 5},
			le:              15,
			isUpperTrim:     true,
			expectedKept:    5,
			expectedRemoved: 0,
		},
		{
			name:            "(-Inf, 10] trim_upper le < upper: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: negInf, Upper: 10, Count: 5},
			le:              5,
			isUpperTrim:     true,
			expectedKept:    0,
			expectedRemoved: 5,
		},
		// TRIM_LOWER (>/) - remove values less than le
		{
			name:            "(-Inf, 10] trim_lower le > -Inf: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: negInf, Upper: 10, Count: 5},
			le:              5,
			isUpperTrim:     false,
			expectedKept:    0,
			expectedRemoved: 5,
		},
		{
			name:            "(-Inf, 10] trim_lower le = -Inf (impossible): keep bucket",
			bucket:          &histogram.Bucket[float64]{Lower: negInf, Upper: 10, Count: 5},
			le:              negInf,
			isUpperTrim:     false,
			expectedKept:    5,
			expectedRemoved: 0,
		},

		// Case 2: Bucket with upper bound [lower, +Inf)
		// TRIM_UPPER (</) - remove values greater than le
		{
			name:            "[10, +Inf) trim_upper le >= lower: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              10,
			isUpperTrim:     true,
			expectedKept:    0,
			expectedRemoved: 5,
		},
		{
			name:            "[10, +Inf) trim_upper le > lower: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              15,
			isUpperTrim:     true,
			expectedKept:    0,
			expectedRemoved: 5,
		},
		{
			name:            "[10, +Inf) trim_upper le < lower: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              5,
			isUpperTrim:     true,
			expectedKept:    0,
			expectedRemoved: 5,
		},
		// TRIM_LOWER (>/) - remove values less than le
		{
			name:            "[10, +Inf) trim_lower le <= lower: keep entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              10,
			isUpperTrim:     false,
			expectedKept:    5,
			expectedRemoved: 0,
		},
		{
			name:            "[10, +Inf) trim_lower le < lower: keep entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              5,
			isUpperTrim:     false,
			expectedKept:    5,
			expectedRemoved: 0,
		},
		{
			name:            "[10, +Inf) trim_lower le > lower: remove entire bucket",
			bucket:          &histogram.Bucket[float64]{Lower: 10, Upper: inf, Count: 5},
			le:              15,
			isUpperTrim:     false,
			expectedKept:    0,
			expectedRemoved: 5,
		},

		// Case 3: Finite bucket [lower, upper] - defers to interpolateBucket
		{
			name:            "[5, 10] finite bucket: defers to interpolateBucket",
			bucket:          &histogram.Bucket[float64]{Lower: 5, Upper: 10, Count: 5},
			le:              7,
			isUpperTrim:     true,
			expectedKept:    0,
			expectedRemoved: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keptCount, removedCount := HandleInfiniteBuckets(tc.bucket, tc.le, tc.isUpperTrim)
			require.Equal(t, tc.expectedKept, keptCount, "unexpected kept count")
			require.Equal(t, tc.expectedRemoved, removedCount, "unexpected removed count")
		})
	}
}
