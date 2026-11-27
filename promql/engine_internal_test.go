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
	"math"
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

func TestSubqueryTimeRange(t *testing.T) {
	// noStepInterval returns the interval the engine uses for a subquery without
	// an explicit step ([range:]). This is the configured global evaluation
	// interval (default 1 minute) and does not depend on the subquery range, so
	// the argument is ignored, matching NoStepSubqueryIntervalFn in production.
	const noStepEvalInterval = time.Minute
	noStepInterval := func(int64) int64 { return durationMilliseconds(noStepEvalInterval) }

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
			// Subquery without an explicit step uses noStepSubqueryIntervalFn,
			// here the 60s default. First aligned start after 201s-100s=101s is
			// 120s; end is aligned down to 216s as above.
			name:        "range query, subquery without explicit step",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[100s:]",
			wantStart: 120 * second, wantEnd: 216 * second, wantInterval: 60 * second,
		},
		{
			// For a non-negative boundary the truncated-down multiple always
			// lands on or below the boundary, so the +interval guard fires here
			// (140s -> 145s) as in every case above. The only way to skip the
			// guard is a negative boundary, exercised in the next case.
			name:        "range query with offset, start bumped onto the grid",
			parentStart: 201 * second, parentEnd: 216 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s] offset 1s",
			wantStart: 145 * second, wantEnd: 215 * second, wantInterval: 5 * second,
		},
		{
			// offset+range exceeds the parent start, so the boundary
			// (30s-61s=-31s) is negative. Integer division truncates toward
			// zero, yielding -30s, which is already strictly past -31s, so the
			// +interval guard is NOT taken. Guards the negative-timestamp path.
			name:        "instant query, negative boundary skips the start bump",
			parentStart: 30 * second, parentEnd: 30 * second, parentInterval: 0,
			query:     "metric[61s:5s]",
			wantStart: -30 * second, wantEnd: 30 * second, wantInterval: 5 * second,
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

			start, end, interval := ev.subqueryTimeRange(sq)

			require.Equal(t, tc.wantStart, start, "subquery start timestamp")
			require.Equal(t, tc.wantEnd, end, "subquery end timestamp")
			require.Equal(t, tc.wantInterval, interval, "subquery interval")
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

func TestCostFallbackSampleWindow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		end       int64
		interval  time.Duration
		wantStart int64
	}{
		{name: "default", end: 0, wantStart: -300000},
		{name: "negative interval", end: 0, interval: -time.Second, wantStart: -300000},
		{name: "minimum duration", end: 0, interval: time.Duration(math.MinInt64), wantStart: -300000},
		{name: "eight intervals", end: 0, interval: time.Minute, wantStart: -480000},
		{name: "maximum duration", end: 0, interval: time.Duration(math.MaxInt64), wantStart: -1800000},
		{name: "timestamp underflow", end: math.MinInt64 + 1, interval: time.Minute, wantStart: math.MinInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := fallbackSampleWindow(tc.end, tc.interval)
			require.Equal(t, tc.wantStart, start)
			require.Equal(t, tc.end, end)
		})
	}
}

func TestFloatWinScanAdvanceResync(t *testing.T) {
	newScanWithPoints := func(n int) *floatWinScan {
		s := &floatWinScan{}
		t.Cleanup(func() {
			putFPointSlice(s.win.buf)
			putWinEventSeqs(s.eventSeqs)
		})
		s.win.reset(nil)
		for i := range n {
			s.win.append(FPoint{T: int64((i + 1) * 1000), F: float64(i)}, 0, nil)
		}
		return s
	}

	t.Run("seriesID mismatch resets state", func(t *testing.T) {
		s := newScanWithPoints(4)
		from, to, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		require.Equal(t, 0, from)
		require.Equal(t, 3, to)
		s.appendEvent(1)
		s.lastOverlapCurrSeq = 2

		s.win.reset(nil)
		s.win.append(FPoint{T: 10_000, F: 1}, 0, nil)
		s.win.append(FPoint{T: 11_000, F: 2}, 0, nil)
		from, to, ok = s.advance(s.win.activeSlice())
		require.True(t, ok)
		require.Equal(t, 0, from)
		require.Equal(t, 1, to)
		require.Empty(t, s.eventSeqs)
		require.Zero(t, s.eventOff)
		require.Zero(t, s.lastOverlapCurrSeq)
	})

	t.Run("nextPairSeq behind startSeq resyncs", func(t *testing.T) {
		s := newScanWithPoints(3)
		_, _, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		s.appendEvent(0)
		s.lastOverlapCurrSeq = 1
		require.Equal(t, 2, s.nextPairSeq)

		// Advance the window past nextPairSeq without calling advance on intermediate steps
		// (as happens while mixed float+histogram windows bypass the fast path).
		s.win.append(FPoint{T: 4000, F: 4}, 0, nil)
		s.win.append(FPoint{T: 5000, F: 5}, 0, nil)
		s.win.drop(3, nil)
		from, to, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		require.Equal(t, 3, from)
		require.Equal(t, 4, to)
		require.Empty(t, s.eventSeqs)
		require.Zero(t, s.lastOverlapCurrSeq)
	})

	t.Run("nextPairSeq past endSeq resyncs", func(t *testing.T) {
		s := newScanWithPoints(3)
		_, _, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		s.appendEvent(1)
		s.nextPairSeq = s.win.endSeq + 1

		from, to, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		require.Equal(t, 0, from)
		require.Equal(t, 2, to)
		require.Empty(t, s.eventSeqs)
		require.Zero(t, s.eventOff)
	})

	t.Run("drained events reset slice offset", func(t *testing.T) {
		s := newScanWithPoints(4)
		_, _, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		s.appendEvent(0)
		s.appendEvent(1)

		s.win.drop(2, nil)
		s.win.append(FPoint{T: 5000, F: 5}, 0, nil)
		from, to, ok := s.advance(s.win.activeSlice())
		require.True(t, ok)
		require.Equal(t, 3, from)
		require.Equal(t, 4, to)
		require.Empty(t, s.eventSeqs)
		require.Zero(t, s.eventOff)
	})

	t.Run("misaligned floats returns false", func(t *testing.T) {
		s := newScanWithPoints(3)
		_, _, ok := s.advance([]FPoint{{T: 1000, F: 0}, {T: 2000, F: 1}})
		require.False(t, ok)

		// Same length but different backing array must also fall back.
		copied := append([]FPoint(nil), s.win.activeSlice()...)
		_, _, ok = s.advance(copied)
		require.False(t, ok)
	})
}

func TestFloatWindowCompactionAndGrowth(t *testing.T) {
	for _, withST := range []bool{false, true} {
		name := "st=false"
		if withST {
			name = "st=true"
		}
		t.Run(name, func(t *testing.T) {
			var empty floatWindow
			require.Nil(t, empty.activeSlice())

			var sts *StartTimestamps
			if withST {
				sts = &StartTimestamps{}
			}

			// Compact in place at the exact off >= n/4 threshold (off=4, n=12, n/4=3).
			w := floatWindow{buf: make([]FPoint, 0, 16)}
			if withST {
				w.stBuf = make([]int64, 0, 16)
			}
			for i := range 16 {
				w.append(FPoint{T: int64(i * 1000), F: float64(i)}, int64(i*100), sts)
			}
			w.drop(4, sts)
			require.Equal(t, 4, w.off)
			require.Equal(t, 4, w.startSeq)
			w.append(FPoint{T: 16_000, F: 16}, 1600, sts)
			require.Zero(t, w.off)
			require.Equal(t, 16, cap(w.buf))
			require.Len(t, w.activeSlice(), 13)
			require.Equal(t, int64(4000), w.activeSlice()[0].T)
			require.Equal(t, int64(16_000), w.activeSlice()[12].T)
			if withST {
				require.Equal(t, 16, cap(w.stBuf))
				require.Len(t, sts.Floats, 13)
				require.Equal(t, int64(400), sts.Floats[0])
				require.Equal(t, int64(1600), sts.Floats[12])
			}

			// Grow when 0 < off < n/4 (off=2, n=14, n/4=3 -> newCap = 14 + max(7, 16) = 30).
			wGrow := floatWindow{buf: make([]FPoint, 0, 16)}
			if withST {
				wGrow.stBuf = make([]int64, 0, 16)
			}
			for i := range 16 {
				wGrow.append(FPoint{T: int64(i * 1000), F: float64(i)}, int64(i*100), sts)
			}
			wGrow.drop(2, sts)
			wGrow.append(FPoint{T: 16_000, F: 16}, 1600, sts)
			require.Zero(t, wGrow.off)
			require.Equal(t, 30, cap(wGrow.buf))
			require.Len(t, wGrow.activeSlice(), 15)
			require.Equal(t, int64(2000), wGrow.activeSlice()[0].T)
			if withST {
				require.Equal(t, 30, cap(wGrow.stBuf))
				require.Len(t, sts.Floats, 15)
				require.Equal(t, int64(200), sts.Floats[0])
				require.Equal(t, int64(1600), sts.Floats[14])
			}

			// Growth when off == 0 lets append grow normally without compacting.
			wZeroOff := floatWindow{buf: make([]FPoint, 0, 16)}
			if withST {
				wZeroOff.stBuf = make([]int64, 0, 16)
			}
			for i := range 17 {
				wZeroOff.append(FPoint{T: int64(i * 1000), F: float64(i)}, int64(i*100), sts)
			}
			require.Zero(t, wZeroOff.off)
			require.Greater(t, cap(wZeroOff.buf), 16)
			require.Len(t, wZeroOff.activeSlice(), 17)
			if withST {
				require.Len(t, sts.Floats, 17)
			}
		})
	}

	t.Run("large pooled buf bounds stBuf capacity", func(t *testing.T) {
		sts := &StartTimestamps{}
		w := floatWindow{buf: make([]FPoint, 0, 5000)}
		for i := range 10 {
			w.append(FPoint{T: int64(i * 1000), F: float64(i)}, int64(i*10), sts)
		}
		for step := 10; step < 210; step++ {
			w.drop(1, sts)
			w.append(FPoint{T: int64(step * 1000), F: float64(step)}, int64(step*10), sts)
			require.Len(t, w.activeSlice(), 10)
			require.Len(t, sts.Floats, 10)
			require.Equal(t, int64((step-9)*1000), w.activeSlice()[0].T)
			require.Equal(t, int64((step-9)*10), sts.Floats[0])
			require.Equal(t, int64(step*10), sts.Floats[9])
		}
		require.Equal(t, 5000, cap(w.buf), "large pooled w.buf should compact in place without reallocating")
		require.LessOrEqual(t, cap(w.stBuf), 32, "w.stBuf capacity should stay bounded by active window size")
	})
}

func TestFloatWinScanAppendEvent(t *testing.T) {
	t.Run("compacts in place when eventOff >= n/4", func(t *testing.T) {
		s := &floatWinScan{eventSeqs: make([]int, 0, 16)}
		for i := range 16 {
			s.appendEvent(i)
		}
		s.eventOff = 4 // n = 12, n/4 = 3 -> compact in place.
		s.appendEvent(16)
		require.Zero(t, s.eventOff)
		require.Equal(t, 16, cap(s.eventSeqs))
		require.Equal(t, []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, s.eventSeqs)
	})

	t.Run("grows to 1.5x when 0 < eventOff < n/4", func(t *testing.T) {
		s := &floatWinScan{eventSeqs: make([]int, 0, 16)}
		for i := range 16 {
			s.appendEvent(i)
		}
		s.eventOff = 2 // n = 14, n/4 = 3 -> grow to 14 + max(7, 16) = 30.
		s.appendEvent(16)
		require.Zero(t, s.eventOff)
		require.Equal(t, 30, cap(s.eventSeqs))
		require.Equal(t, []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, s.eventSeqs)
	})

	t.Run("compacts when all prior events evicted (n == 0)", func(t *testing.T) {
		s := &floatWinScan{eventSeqs: make([]int, 0, 16)}
		for i := range 16 {
			s.appendEvent(i)
		}
		s.eventOff = 16
		s.appendEvent(100)
		require.Zero(t, s.eventOff)
		require.Equal(t, 16, cap(s.eventSeqs))
		require.Equal(t, []int{100}, s.eventSeqs)
	})

	t.Run("putWinEventSeqs drops oversized slices", func(t *testing.T) {
		putWinEventSeqs(make([]int, 0, maxWinEventSeqsPoolCap+1))
		got := getWinEventSeqs(16)
		require.LessOrEqual(t, cap(got), maxWinEventSeqsPoolCap)
	})
}
