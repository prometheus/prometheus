// Copyright The Prometheus Authors
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
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/annotations"
)

var testParser = parser.NewParser(parser.Options{})

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var output bytes.Buffer
	logger := promslog.New(&promslog.Config{Writer: &output})
	ev := &evaluator{logger: logger}

	expr, _ := testParser.ParseExpr("sum(up)")

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
