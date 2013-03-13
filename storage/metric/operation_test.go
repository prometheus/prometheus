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
	"github.com/prometheus/prometheus/utility/test"
	"sort"
	"testing"
	"time"
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
						time: testInstant,
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
			},
			// Single range; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Single interval; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Duplicate points; return single point.
			{
				in: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
			},
			// Duplicate ranges; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Duplicate intervals; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate interval; return master.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate range; return master.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
			},
			// Equal range with different interval; return both.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Different range with different interval; return best.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Include Truncated Intervals with Range.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(30 * time.Second),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
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
						from:     testInstant,
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
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
						from:     testInstant,
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAtIntervalOp{
						from:     testInstant.Add(2 * time.Minute),
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(scenario.in)

		out = optimizeTimeGroups(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.time.Unix() != actual.time.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.time, actual.time)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
						from:     testInstant.Add(1 * time.Minute),
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Compact Ranges with Subservient Range
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Carving Middle Elements
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(2 * time.Minute),
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(2 * time.Minute),
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						// Since the range operation consumes Now() + 3 Minutes, we start
						// an additional ten seconds later.
						from:     testInstant.Add(3 * time.Minute).Add(10 * time.Second),
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
						time: testInstant.Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(1 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(2 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(3 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(4 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(5 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(6 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(5 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant.Add(30 * time.Second),
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(5 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(6 * time.Minute).Add(30 * time.Second),
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(scenario.in)

		out = optimizeForward(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.time.Unix() != actual.time.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.time, actual.time)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
						time: testInstant,
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
			},
			// Single range; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Single interval; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Duplicate points; return single point.
			{
				in: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant,
					},
				},
			},
			// Duplicate ranges; return single range.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
				},
			},
			// Duplicate intervals; return single interval.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate interval; return master.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
			},
			// Subordinate range; return master.
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(1 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
			},
			// Equal range with different interval; return both.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Different range with different interval; return best.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 5,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Include Truncated Intervals with Range.
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(1 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(30 * time.Second),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
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
						from:     testInstant,
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
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
						from:     testInstant,
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAtIntervalOp{
						from:     testInstant.Add(2 * time.Minute),
						through:  testInstant.Add(3 * time.Minute),
						interval: time.Second * 10,
					},
				},
			},
			// Compact Interval with Subservient Range
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant.Add(1 * time.Minute),
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Compact Ranges with Subservient Range
			{
				in: ops{
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(2 * time.Minute),
					},
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAlongRangeOp{
						from:    testInstant,
						through: testInstant.Add(3 * time.Minute),
					},
				},
			},
			// Carving Middle Elements
			{
				in: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(5 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(2 * time.Minute),
						through: testInstant.Add(3 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtIntervalOp{
						from:     testInstant,
						through:  testInstant.Add(2 * time.Minute),
						interval: time.Second * 10,
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(2 * time.Minute),
						through: testInstant.Add(3 * time.Minute),
					},
					&getValuesAtIntervalOp{
						// Since the range operation consumes Now() + 3 Minutes, we start
						// an additional ten seconds later.
						from:     testInstant.Add(3 * time.Minute).Add(10 * time.Second),
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
						time: testInstant.Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(1 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(2 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(3 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(4 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(5 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(6 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(5 * time.Minute),
					},
				},
				out: ops{
					&getValuesAtTimeOp{
						time: testInstant.Add(30 * time.Second),
					},
					&getValuesAlongRangeOp{
						from:    testInstant.Add(1 * time.Minute),
						through: testInstant.Add(5 * time.Minute),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(5 * time.Minute).Add(30 * time.Second),
					},
					&getValuesAtTimeOp{
						time: testInstant.Add(6 * time.Minute).Add(30 * time.Second),
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		// The compaction system assumes that values are sorted on input.
		sort.Sort(scenario.in)

		out = optimize(scenario.in)

		if len(out) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(out))
		}

		for j, op := range out {

			if actual, ok := op.(*getValuesAtTimeOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtTimeOp); ok {
					if expected.time.Unix() != actual.time.Unix() {
						t.Fatalf("%d.%d. expected time %s, got %s", i, j, expected.time, actual.time)
					}
				} else {
					t.Fatalf("%d.%d. expected getValuesAtTimeOp, got %s", i, j, actual)
				}

			} else if actual, ok := op.(*getValuesAtIntervalOp); ok {

				if expected, ok := scenario.out[j].(*getValuesAtIntervalOp); ok {
					// Shaving off nanoseconds.
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
					if expected.from.Unix() != actual.from.Unix() {
						t.Fatalf("%d.%d. expected from %s, got %s", i, j, expected.from, actual.from)
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
