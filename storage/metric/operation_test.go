// Copyright 2013 Prometheus Team
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

package metric

import (
	"sort"
	"testing"
	"time"

	"github.com/prometheus/prometheus/utility/test"
)

func testOptimizeTimeGroups(t test.Tester) {
	var (
		out ops

		scenarios = []struct {
			in  ops
			out ops
		}{
			// Empty set; return empty set.
			{
				in:  ops{},
				out: ops{},
			},
			// Single time; return single time.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
			},
			// Single range; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Single interval; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Duplicate points; return single point.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
			},
			// Duplicate ranges; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Duplicate intervals; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate interval; return master.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate range; return master.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(2 * time.Minute),
					},
				},
			},
			// Equal range with different interval; return both.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Different range with different interval; return best.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			/*
				// Include Truncated Intervals with Range.
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(1 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(30 * time.Second),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(30 * time.Second),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(30 * time.Second),
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
				// Compacted Forward Truncation
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(2 * time.Minute),
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
				// Compacted Tail Truncation
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(2 * time.Minute),
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
			*/
			// Regression Validation 1: Multiple Overlapping Interval Requests
			//                          This one specific case expects no mutation.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(15 * time.Second)},
						through: testInstant.Add(15*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(30 * time.Second)},
						through: testInstant.Add(30*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(45 * time.Second)},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(15 * time.Second)},
						through: testInstant.Add(15*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(30 * time.Second)},
						through: testInstant.Add(30*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(45 * time.Second)},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(currentTimeSort{scenario.in})

		out = optimizeTimeGroups(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.current, actual.current)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
					if expected.interval != (actual.interval) {
						t.Fatalf("%d.%d. expected interval %s, got %s", i, j, expected.interval, actual.interval)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtIntervalOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAlongRangeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAlongRangeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAlongRangeOp, got %s", i, j, actual)
				}

			}

		}
	}
}

func TestOptimizeTimeGroups(t *testing.T) {
	testOptimizeTimeGroups(t)
}

func BenchmarkOptimizeTimeGroups(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testOptimizeTimeGroups(b)
	}
}

func testOptimizeForward(t test.Tester) {
	var (
		out ops

		scenarios = []struct {
			in  ops
			out ops
		}{
			// Compact Interval with Subservient Range
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant.Add(1 * time.Minute)},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Compact Ranges with Subservient Range
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Carving Middle Elements
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						// Since the range operation consumes Now() + 3 Minutes, we start
						// an additional ten seconds later.
						baseOp:   baseOp{current: testInstant.Add(3*time.Minute + 10*time.Second)},
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Compact Subservient Points with Range
			// The points are at half-minute offsets due to optimizeTimeGroups
			// work.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(30 * time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(1*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(2*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(3*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(4*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(5*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(6*time.Minute + 30*time.Second)},
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(5 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(30 * time.Second)},
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(5*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(6*time.Minute + 30*time.Second)},
					},
				},
			},
			// Regression Validation 1: Multiple Overlapping Interval Requests
			//                          We expect to find compaction.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(15 * time.Second)},
						through: testInstant.Add(15*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(30 * time.Second)},
						through: testInstant.Add(30*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(45 * time.Second)},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
			},
			// Range with subsequent overlapping interval.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant.Add(1 * time.Minute)},
						through:  testInstant.Add(4 * time.Minute),
						interval: time.Second * 10,
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant.Add(3*time.Minute + 10*time.Second)},
						through:  testInstant.Add(4 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(currentTimeSort{scenario.in})

		out = optimizeForward(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.current, actual.current)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
					if expected.interval != (actual.interval) {
						t.Fatalf("%d.%d. expected interval %s, got %s", i, j, expected.interval, actual.interval)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtIntervalOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAlongRangeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAlongRangeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAlongRangeOp, got %s", i, j, actual)
				}

			}

		}
	}
}

func TestOptimizeForward(t *testing.T) {
	testOptimizeForward(t)
}

func BenchmarkOptimizeForward(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testOptimizeForward(b)
	}
}

func testOptimize(t test.Tester) {
	var (
		out ops

		scenarios = []struct {
			in  ops
			out ops
		}{
			// Empty set; return empty set.
			{
				in:  ops{},
				out: ops{},
			},
			// Single time; return single time.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
			},
			// Single range; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Single interval; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Duplicate points; return single point.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant},
					},
				},
			},
			// Duplicate ranges; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Duplicate intervals; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate interval; return master.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate range; return master.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(2 * time.Minute),
					},
				},
			},
			// Equal range with different interval; return both.
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			/*
				// Different range with different interval; return best.
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(1 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(1 * time.Minute),
							interval: time.Second * 5,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 5,
						},
					},
					out: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 5,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
				// Include Truncated Intervals with Range.
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(1 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(30 * time.Second),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(30 * time.Second),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(30 * time.Second),
							through:  testInstant.Add(2 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
				// Compacted Forward Truncation
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(2 * time.Minute),
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
				// Compacted Tail Truncation
				{
					in: ops{
						&getValuesAtIntervalOp{
							baseOp: baseOp{current: testInstant},
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
					},
					out: ops{
						&getValuesAlongRangeOp{
							baseOp: baseOp{current: testInstant},
							through: testInstant.Add(2 * time.Minute),
						},
						&getValuesAtIntervalOp{
							from:     testInstant.Add(2 * time.Minute),
							through:  testInstant.Add(3 * time.Minute),
							interval: time.Second * 10,
						},
					},
				},
			*/
			// Compact Interval with Subservient Range
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant.Add(1 * time.Minute)},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Compact Ranges with Subservient Range
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Carving Middle Elements
			{
				in: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						baseOp:   baseOp{current: testInstant},
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						// Since the range operation consumes Now() + 3 Minutes, we start
						// an additional ten seconds later.
						baseOp:   baseOp{current: testInstant.Add(3*time.Minute + 10*time.Second)},
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Compact Subservient Points with Range
			// The points are at half-minute offsets due to optimizeTimeGroups
			// work.
			{
				in: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(30 * time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(1*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(2*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(3*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(4*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(5*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(6*time.Minute + 30*time.Second)},
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(5 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(30 * time.Second)},
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(5*time.Minute + 30*time.Second)},
					},
					&getValuesAtTimeOp{
						baseOp: baseOp{current: testInstant.Add(6*time.Minute + 30*time.Second)},
					},
				},
			},
			// Regression Validation 1: Multiple Overlapping Interval Requests
			//                          We expect to find compaction.
			{
				in: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(15 * time.Second)},
						through: testInstant.Add(15*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(30 * time.Second)},
						through: testInstant.Add(30*time.Second + 5*time.Minute),
					},
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant.Add(45 * time.Second)},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						baseOp:  baseOp{current: testInstant},
						through: testInstant.Add(45*time.Second + 5*time.Minute),
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(currentTimeSort{scenario.in})

		out = optimize(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.current, actual.current)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
					if expected.interval != (actual.interval) {
						t.Fatalf("%d.%d. expected interval %s, got %s", i, j, expected.interval, actual.interval)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtIntervalOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAlongRangeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAlongRangeOp); ok {
					if expected.current.Unix() != actual.current.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.current, actual.current)
					}
					if expected.through.Unix() != actual.through.Unix() {
						t.Fatalf("%d.%d. expected through %s, got %s", i, j, expected.through, actual.through)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAlongRangeOp, got %s", i, j, actual)
				}

			}

		}
	}
}

func TestOptimize(t *testing.T) {
	testOptimize(t)
}

func BenchmarkOptimize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testOptimize(b)
	}
}

func TestGetValuesAtTimeOp(t *testing.T) {
	var scenarios = []struct {
		op  getValuesAtTimeOp
		in  Values
		out Values
	}{
		// No values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant},
			},
		},
		// Operator time before single value.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time exactly at single value.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(1 * time.Minute)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time after single value.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(2 * time.Minute)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time before two values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time at first of two values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(1 * time.Minute)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time between first and second of two values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(90 * time.Second)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time at second of two values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(2 * time.Minute)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator time after second of two values.
		{
			op: getValuesAtTimeOp{
				baseOp: baseOp{current: testInstant.Add(3 * time.Minute)},
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
	}

	for i, scenario := range scenarios {
		actual := scenario.op.ExtractSamples(scenario.in)
		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. expected length %d, got %d: %v", i, len(scenario.out), len(actual), scenario.op)
			t.Fatalf("%d. expected length %d, got %d", i, len(scenario.out), len(actual))
		}
		for j, out := range scenario.out {
			if !out.Equal(actual[j]) {
				t.Fatalf("%d. expected output %v, got %v", i, scenario.out, actual)
			}
		}
	}
}

func TestGetValuesAtIntervalOp(t *testing.T) {
	var scenarios = []struct {
		op  getValuesAtIntervalOp
		in  Values
		out Values
	}{
		// No values.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant},
				through:  testInstant.Add(1 * time.Minute),
				interval: 30 * time.Second,
			},
		},
		// Entire operator range before first value.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant},
				through:  testInstant.Add(1 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator range starts before first value, ends within available values.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant},
				through:  testInstant.Add(2 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
		},
		// Entire operator range is within available values.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant.Add(1 * time.Minute)},
				through:  testInstant.Add(2 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator range begins before first value, ends after last.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant},
				through:  testInstant.Add(3 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator range begins within available values, ends after the last value.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant.Add(2 * time.Minute)},
				through:  testInstant.Add(4 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
		},
		// Entire operator range after the last available value.
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant.Add(2 * time.Minute)},
				through:  testInstant.Add(3 * time.Minute),
				interval: 30 * time.Second,
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator interval skips over several values and ends past the last
		// available value. This is to verify that we still include the last value
		// of a series even if we target a time past it and haven't extracted that
		// value yet as part of a previous interval step (thus the necessity to
		// skip over values for the test).
		{
			op: getValuesAtIntervalOp{
				baseOp:   baseOp{current: testInstant.Add(30 * time.Second)},
				through:  testInstant.Add(4 * time.Minute),
				interval: 3 * time.Minute,
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
		},
	}
	for i, scenario := range scenarios {
		actual := scenario.op.ExtractSamples(scenario.in)
		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. expected length %d, got %d: %v", i, len(scenario.out), len(actual), actual)
		}

		if len(scenario.in) < 1 {
			continue
		}
		lastExtractedTime := scenario.out[len(scenario.out)-1].Timestamp
		if !scenario.op.Consumed() && scenario.op.CurrentTime().Before(lastExtractedTime) {
			t.Fatalf("%d. expected op to be consumed or with CurrentTime() after current chunk, %v, %v", i, scenario.op.CurrentTime(), scenario.out)
		}

		for j, out := range scenario.out {
			if !out.Equal(actual[j]) {
				t.Fatalf("%d. expected output %v, got %v", i, scenario.out, actual)
			}
		}
	}
}

func TestGetValuesAlongRangeOp(t *testing.T) {
	var scenarios = []struct {
		op  getValuesAlongRangeOp
		in  Values
		out Values
	}{
		// No values.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant},
				through: testInstant.Add(1 * time.Minute),
			},
		},
		// Entire operator range before first value.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant},
				through: testInstant.Add(1 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{},
		},
		// Operator range starts before first value, ends within available values.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant},
				through: testInstant.Add(2 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Entire operator range is within available values.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant.Add(1 * time.Minute)},
				through: testInstant.Add(2 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator range begins before first value, ends after last.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant},
				through: testInstant.Add(3 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
			},
		},
		// Operator range begins within available values, ends after the last value.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
				through: testInstant.Add(4 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(2 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(3 * time.Minute),
					Value:     1,
				},
			},
		},
		// Entire operator range after the last available value.
		{
			op: getValuesAlongRangeOp{
				baseOp:  baseOp{current: testInstant.Add(2 * time.Minute)},
				through: testInstant.Add(3 * time.Minute),
			},
			in: Values{
				{
					Timestamp: testInstant,
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(1 * time.Minute),
					Value:     1,
				},
			},
			out: Values{},
		},
	}
	for i, scenario := range scenarios {
		actual := scenario.op.ExtractSamples(scenario.in)
		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. expected length %d, got %d: %v", i, len(scenario.out), len(actual), actual)
		}
		for j, out := range scenario.out {
			if !out.Equal(actual[j]) {
				t.Fatalf("%d. expected output %v, got %v", i, scenario.out, actual)
			}
		}
	}
}

func TestGetValueRangeAtIntervalOp(t *testing.T) {
	testOp := getValueRangeAtIntervalOp{
		baseOp:        baseOp{current: testInstant.Add(-2 * time.Minute)},
		rangeThrough:  testInstant,
		rangeDuration: 2 * time.Minute,
		interval:      10 * time.Minute,
		through:       testInstant.Add(20 * time.Minute),
	}

	var scenarios = []struct {
		op  getValueRangeAtIntervalOp
		in  Values
		out Values
	}{
		// All values before the first range.
		{
			op: testOp,
			in: Values{
				{
					Timestamp: testInstant.Add(-4 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(-3 * time.Minute),
					Value:     2,
				},
			},
			out: Values{},
		},
		// Values starting before first range, ending after last.
		{
			op: testOp,
			in: Values{
				{
					Timestamp: testInstant.Add(-4 * time.Minute),
					Value:     1,
				},
				{
					Timestamp: testInstant.Add(-3 * time.Minute),
					Value:     2,
				},
				{
					Timestamp: testInstant.Add(-2 * time.Minute),
					Value:     3,
				},
				{
					Timestamp: testInstant.Add(-1 * time.Minute),
					Value:     4,
				},
				{
					Timestamp: testInstant.Add(0 * time.Minute),
					Value:     5,
				},
				{
					Timestamp: testInstant.Add(5 * time.Minute),
					Value:     6,
				},
				{
					Timestamp: testInstant.Add(8 * time.Minute),
					Value:     7,
				},
				{
					Timestamp: testInstant.Add(9 * time.Minute),
					Value:     8,
				},
				{
					Timestamp: testInstant.Add(10 * time.Minute),
					Value:     9,
				},
				{
					Timestamp: testInstant.Add(15 * time.Minute),
					Value:     10,
				},
				{
					Timestamp: testInstant.Add(18 * time.Minute),
					Value:     11,
				},
				{
					Timestamp: testInstant.Add(19 * time.Minute),
					Value:     12,
				},
				{
					Timestamp: testInstant.Add(20 * time.Minute),
					Value:     13,
				},
				{
					Timestamp: testInstant.Add(21 * time.Minute),
					Value:     14,
				},
			},
			out: Values{
				{
					Timestamp: testInstant.Add(-2 * time.Minute),
					Value:     3,
				},
				{
					Timestamp: testInstant.Add(-1 * time.Minute),
					Value:     4,
				},
				{
					Timestamp: testInstant.Add(0 * time.Minute),
					Value:     5,
				},
				{
					Timestamp: testInstant.Add(8 * time.Minute),
					Value:     7,
				},
				{
					Timestamp: testInstant.Add(9 * time.Minute),
					Value:     8,
				},
				{
					Timestamp: testInstant.Add(10 * time.Minute),
					Value:     9,
				},
				{
					Timestamp: testInstant.Add(18 * time.Minute),
					Value:     11,
				},
				{
					Timestamp: testInstant.Add(19 * time.Minute),
					Value:     12,
				},
				{
					Timestamp: testInstant.Add(20 * time.Minute),
					Value:     13,
				},
			},
		},
		// Values starting after last range.
		{
			op: testOp,
			in: Values{
				{
					Timestamp: testInstant.Add(21 * time.Minute),
					Value:     14,
				},
			},
			out: Values{},
		},
	}
	for i, scenario := range scenarios {
		actual := Values{}
		for !scenario.op.Consumed() {
			actual = append(actual, scenario.op.ExtractSamples(scenario.in)...)
		}
		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. expected length %d, got %d: %v", i, len(scenario.out), len(actual), actual)
		}
		for j, out := range scenario.out {
			if !out.Equal(actual[j]) {
				t.Fatalf("%d. expected output %v, got %v", i, scenario.out, actual)
			}
		}
	}
}
