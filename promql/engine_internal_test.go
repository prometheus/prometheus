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
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/stats"
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

func TestNewSubqueryEvaluator(t *testing.T) {
	// noStepInterval mimics the default step PromQL picks for a subquery without
	// an explicit step ([range:]): one tenth of the range, clamped to >= 1ms.
	noStepInterval := func(rangeMillis int64) int64 {
		if step := rangeMillis / 10; step > 0 {
			return step
		}
		return 1
	}

	const second = 1000

	for _, tc := range []struct {
		name string
		// Parent evaluator (the outer range/instant query) time range.
		parentStart, parentEnd, parentInterval int64
		query                                  string

		wantStart, wantEnd, wantInterval int64
	}{
		{
			// Instant query: a single evaluation step, no end alignment needed.
			// Subquery steps span (1000s-60s, 1000s] on the 5s grid: 945s..1000s.
			name:        "instant query",
			parentStart: 1000 * second, parentEnd: 1000 * second, parentInterval: 0,
			query:     "metric[60s:5s]",
			wantStart: 945 * second, wantEnd: 1000 * second, wantInterval: 5 * second,
		},
		{
			// Range query whose end is already on the 5s step grid
			// (steps at 201s, 206s, 211s, 216s): subquery ends at the parent end.
			name:        "range query, end already step-aligned",
			parentStart: 201 * second, parentEnd: 216 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s]",
			wantStart: 145 * second, wantEnd: 216 * second, wantInterval: 5 * second,
		},
		{
			// Same query but end=220s is 4s past the last aligned step (216s).
			// The subquery must still stop at 216s, matching the aligned case.
			name:        "range query, end past last step is aligned down",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s]",
			wantStart: 145 * second, wantEnd: 216 * second, wantInterval: 5 * second,
		},
		{
			// Offset shifts both the subquery start and end back by the offset.
			name:        "range query with offset, end aligned down",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s] offset 30s",
			wantStart: 115 * second, wantEnd: 186 * second, wantInterval: 5 * second,
		},
		{
			// Subquery without an explicit step uses noStepSubqueryIntervalFn.
			name:        "range query, subquery without explicit step",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[100s:]",
			wantStart: 110 * second, wantEnd: 216 * second, wantInterval: 10 * second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := testParser.ParseExpr(tc.query)
			require.NoError(t, err)
			// Mirror the engine's query setup: PreprocessExpr resolves
			// duration expressions, then setOffsetForAtModifier resolves
			// OriginalOffset into the Offset the evaluator uses.
			step := time.Duration(tc.parentInterval) * time.Millisecond
			expr, err = PreprocessExpr(expr, timestamp.Time(tc.parentStart), timestamp.Time(tc.parentEnd), step)
			require.NoError(t, err)
			setOffsetForAtModifier(tc.parentStart, expr)
			sq, ok := expr.(*parser.SubqueryExpr)
			require.True(t, ok, "query must parse to a subquery expression")

			ev := &evaluator{
				startTimestamp:           tc.parentStart,
				endTimestamp:             tc.parentEnd,
				interval:                 tc.parentInterval,
				samplesStats:             stats.NewQuerySamples(false),
				noStepSubqueryIntervalFn: noStepInterval,
			}

			newEv := ev.newSubqueryEvaluator(sq)

			require.Equal(t, tc.wantStart, newEv.startTimestamp, "subquery start timestamp")
			require.Equal(t, tc.wantEnd, newEv.endTimestamp, "subquery end timestamp")
			require.Equal(t, tc.wantInterval, newEv.interval, "subquery interval")
		})
	}
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
