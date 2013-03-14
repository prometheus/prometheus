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
	"fmt"
	"github.com/prometheus/prometheus/model"
	"math"
	"sort"
	"time"
)

// Encapsulates a primitive query operation.
type op interface {
	// The time at which this operation starts.
	StartsAt() time.Time
	// Extract samples from stream of values and advance operation time.
	ExtractSamples(in []model.SamplePair) (out []model.SamplePair)
	// Get current operation time or nil if no subsequent work associated with
	// this operator remains.
	CurrentTime() *time.Time
}

// Provides a sortable collection of operations.
type ops []op

func (o ops) Len() int {
	return len(o)
}

func (o ops) Less(i, j int) bool {
	return o[i].StartsAt().Before(o[j].StartsAt())
}

func (o ops) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// Encapsulates getting values at or adjacent to a specific time.
type getValuesAtTimeOp struct {
	time     time.Time
	consumed bool
}

func (o getValuesAtTimeOp) String() string {
	return fmt.Sprintf("getValuesAtTimeOp at %s", o.time)
}

func (g getValuesAtTimeOp) StartsAt() time.Time {
	return g.time
}

func (g *getValuesAtTimeOp) ExtractSamples(in []model.SamplePair) (out []model.SamplePair) {
	if len(in) == 0 {
		return
	}
	out = extractValuesAroundTime(g.time, in)
	g.consumed = true
	return
}

func extractValuesAroundTime(t time.Time, in []model.SamplePair) (out []model.SamplePair) {
	i := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(t)
	})
	fmt.Printf("I: %d\n", i)
	switch i {
	case len(in):
		out = in[len(in)-1:]
	case 0:
		out = append(out, in[0:1]...)
	default:
		out = append(out, in[i-1:i+1]...)
	}
	return
}

func (g getValuesAtTimeOp) CurrentTime() (currentTime *time.Time) {
	if !g.consumed {
		currentTime = &g.time
	}
	return
}

// Encapsulates getting values at a given interval over a duration.
type getValuesAtIntervalOp struct {
	from     time.Time
	through  time.Time
	interval time.Duration
}

func (o getValuesAtIntervalOp) String() string {
	return fmt.Sprintf("getValuesAtIntervalOp from %s each %s through %s", o.from, o.interval, o.through)
}

func (g getValuesAtIntervalOp) StartsAt() time.Time {
	return g.from
}

func (g getValuesAtIntervalOp) Through() time.Time {
	return g.through
}

func (g *getValuesAtIntervalOp) ExtractSamples(in []model.SamplePair) (out []model.SamplePair) {
	if len(in) == 0 {
		return
	}
	lastChunkTime := in[len(in)-1].Timestamp
	for {
		out = extractValuesAroundTime(g.from, in)
		g.from = g.from.Add(g.interval)
		if g.from.After(lastChunkTime) {
			break
		}
		if g.from.After(g.through) {
			break
		}
	}
	return
}

func (g getValuesAtIntervalOp) CurrentTime() (currentTime *time.Time) {
	if g.from.After(g.through) {
		return
	}
	return &g.from
}

type getValuesAlongRangeOp struct {
	from    time.Time
	through time.Time
}

func (o getValuesAlongRangeOp) String() string {
	return fmt.Sprintf("getValuesAlongRangeOp from %s through %s", o.from, o.through)
}

func (g getValuesAlongRangeOp) StartsAt() time.Time {
	return g.from
}

func (g getValuesAlongRangeOp) Through() time.Time {
	return g.through
}

func (g *getValuesAlongRangeOp) ExtractSamples(in []model.SamplePair) (out []model.SamplePair) {
	if len(in) == 0 {
		return
	}
	// Find the first sample where time >= g.from.
	firstIdx := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(g.from)
	})
	if firstIdx == len(in) {
		// No samples at or after operator start time.
		return
	}

	// Find the first sample where time > g.through.
	lastIdx := sort.Search(len(in), func(i int) bool {
		return in[i].Timestamp.After(g.through)
	})
	if lastIdx == firstIdx {
		return
	}

	lastSampleTime := in[lastIdx-1].Timestamp
	g.from = lastSampleTime.Add(time.Duration(1))
	return in[firstIdx:lastIdx]
}

func (g getValuesAlongRangeOp) CurrentTime() (currentTime *time.Time) {
	if g.from.After(g.through) {
		return
	}
	return &g.from
}

// Provides a collection of getMetricRangeOperation.
type getMetricRangeOperations []*getValuesAlongRangeOp

func (s getMetricRangeOperations) Len() int {
	return len(s)
}

func (s getMetricRangeOperations) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Sorts getMetricRangeOperation according to duration in descending order.
type rangeDurationSorter struct {
	getMetricRangeOperations
}

func (s rangeDurationSorter) Less(i, j int) bool {
	l := s.getMetricRangeOperations[i]
	r := s.getMetricRangeOperations[j]

	return !l.through.Before(r.through)
}

// Encapsulates a general operation that occurs over a duration.
type durationOperator interface {
	op

	Through() time.Time
}

// Sorts durationOperator by the operation's duration in ascending order.
type durationOperators []durationOperator

func (o durationOperators) Len() int {
	return len(o)
}

func (o durationOperators) Less(i, j int) bool {
	return o[i].Through().Before(o[j].Through())
}

func (o durationOperators) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// Contains getValuesAtIntervalOp operations.
type getValuesAtIntervalOps []*getValuesAtIntervalOp

func (s getValuesAtIntervalOps) Len() int {
	return len(s)
}

func (s getValuesAtIntervalOps) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Sorts durationOperator by the operation's duration in descending order.
type intervalDurationSorter struct {
	getValuesAtIntervalOps
}

func (s intervalDurationSorter) Less(i, j int) bool {
	l := s.getValuesAtIntervalOps[i]
	r := s.getValuesAtIntervalOps[j]

	return !l.through.Before(r.through)
}

// Sorts getValuesAtIntervalOp operations in ascending order by their
// frequency.
type frequencySorter struct {
	getValuesAtIntervalOps
}

func (s frequencySorter) Less(i, j int) bool {
	l := s.getValuesAtIntervalOps[i]
	r := s.getValuesAtIntervalOps[j]

	return l.interval < r.interval
}

// Selects and returns all operations that are getValuesAtIntervalOps operations.
func collectIntervals(ops ops) (intervals map[time.Duration]getValuesAtIntervalOps) {
	intervals = make(map[time.Duration]getValuesAtIntervalOps)

	for _, operation := range ops {
		switch t := operation.(type) {
		case *getValuesAtIntervalOp:
			operations, _ := intervals[t.interval]

			operations = append(operations, t)
			intervals[t.interval] = operations
		}
	}

	for _, operations := range intervals {
		sort.Sort(intervalDurationSorter{operations})
	}

	return
}

// Selects and returns all operations that are getValuesAlongRangeOp operations.
func collectRanges(ops ops) (ranges getMetricRangeOperations) {
	for _, operation := range ops {
		switch t := operation.(type) {
		case *getValuesAlongRangeOp:
			ranges = append(ranges, t)
		}
	}

	sort.Sort(rangeDurationSorter{ranges})

	return
}

// optimizeForward iteratively scans operations and peeks ahead to subsequent
// ones to find candidates that can either be removed or truncated through
// simplification.  For instance, if a range query happens to overlap a get-a-
// value-at-a-certain-point-request, the range query should flatten and subsume
// the other.
func optimizeForward(pending ops) (out ops) {
	if len(pending) == 0 {
		return
	}

	var (
		firstOperation = pending[0]
	)

	pending = pending[1:len(pending)]

	switch t := firstOperation.(type) {
	case *getValuesAtTimeOp:
		out = ops{firstOperation}
		tail := optimizeForward(pending)

		return append(out, tail...)

	case *getValuesAtIntervalOp:
		// If the last value was a scan at a given frequency along an interval,
		// several optimizations may exist.
		for _, peekOperation := range pending {
			if peekOperation.StartsAt().After(t.Through()) {
				break
			}

			// If the type is not a range request, we can't do anything.
			switch next := peekOperation.(type) {
			case *getValuesAlongRangeOp:
				if !next.Through().After(t.Through()) {
					var (
						before = getValuesAtIntervalOp(*t)
						after  = getValuesAtIntervalOp(*t)
					)

					before.through = next.from

					// Truncate the get value at interval request if a range request cuts
					// it off somewhere.
					var (
						from = next.from
					)

					for {
						from = from.Add(t.interval)

						if from.After(next.through) {
							after.from = from
							break
						}
					}

					pending = append(ops{&before, &after}, pending...)
					sort.Sort(pending)

					return optimizeForward(pending)
				}
			}
		}

	case *getValuesAlongRangeOp:
		for _, peekOperation := range pending {
			if peekOperation.StartsAt().After(t.Through()) {
				break
			}

			switch next := peekOperation.(type) {
			// All values at a specific time may be elided into the range query.
			case *getValuesAtTimeOp:
				pending = pending[1:len(pending)]
				continue
			case *getValuesAlongRangeOp:
				// Range queries should be concatenated if they overlap.
				pending = pending[1:len(pending)]

				if next.Through().After(t.Through()) {
					t.through = next.through

					var (
						head = ops{t}
						tail = pending
					)

					pending = append(head, tail...)

					return optimizeForward(pending)
				}
			case *getValuesAtIntervalOp:
				pending = pending[1:len(pending)]

				if next.through.After(t.Through()) {
					var (
						t = next.from
					)
					for {
						t = t.Add(next.interval)

						if t.After(next.through) {
							next.from = t

							pending = append(ops{next}, pending...)

							return optimizeForward(pending)
						}
					}
				}
			default:
				panic("unknown operation type")
			}
		}
	default:
		panic("unknown operation type")
	}

	// Strictly needed?
	sort.Sort(pending)

	tail := optimizeForward(pending)

	return append(ops{firstOperation}, tail...)
}

func selectQueriesForTime(time time.Time, queries ops) (out ops) {
	if len(queries) == 0 {
		return
	}

	if !queries[0].StartsAt().Equal(time) {
		return
	}

	out = append(out, queries[0])
	tail := selectQueriesForTime(time, queries[1:len(queries)])

	return append(out, tail...)
}

// Flattens queries that occur at the same time according to duration and level
// of greed.
func optimizeTimeGroup(group ops) (out ops) {
	var (
		rangeOperations    = collectRanges(group)
		intervalOperations = collectIntervals(group)

		greediestRange     durationOperator
		greediestIntervals map[time.Duration]durationOperator
	)

	if len(rangeOperations) > 0 {
		operations := durationOperators{}
		for i := 0; i < len(rangeOperations); i++ {
			operations = append(operations, rangeOperations[i])
		}

		// intervaledOperations sorts on the basis of the length of the window.
		sort.Sort(operations)

		greediestRange = operations[len(operations)-1 : len(operations)][0]
	}

	if len(intervalOperations) > 0 {
		greediestIntervals = make(map[time.Duration]durationOperator)

		for i, ops := range intervalOperations {
			operations := durationOperators{}
			for j := 0; j < len(ops); j++ {
				operations = append(operations, ops[j])
			}

			// intervaledOperations sorts on the basis of the length of the window.
			sort.Sort(operations)

			greediestIntervals[i] = operations[len(operations)-1 : len(operations)][0]
		}
	}

	var (
		containsRange    = greediestRange != nil
		containsInterval = len(greediestIntervals) > 0
	)

	if containsRange && !containsInterval {
		out = append(out, greediestRange)
	} else if !containsRange && containsInterval {
		intervalOperations := getValuesAtIntervalOps{}
		for _, o := range greediestIntervals {
			intervalOperations = append(intervalOperations, o.(*getValuesAtIntervalOp))
		}

		sort.Sort(frequencySorter{intervalOperations})

		for _, o := range intervalOperations {
			out = append(out, o)
		}
	} else if containsRange && containsInterval {
		out = append(out, greediestRange)
		for _, op := range greediestIntervals {
			if !op.Through().After(greediestRange.Through()) {
				continue
			}

			// The range operation does not exceed interval.  Leave a snippet of
			// interval.
			var (
				truncated            = op.(*getValuesAtIntervalOp)
				newIntervalOperation getValuesAtIntervalOp
				// Refactor
				remainingSlice    = greediestRange.Through().Sub(greediestRange.StartsAt()) / time.Second
				nextIntervalPoint = time.Duration(math.Ceil(float64(remainingSlice)/float64(truncated.interval)) * float64(truncated.interval/time.Second))
				nextStart         = greediestRange.Through().Add(nextIntervalPoint)
			)

			newIntervalOperation.from = nextStart
			newIntervalOperation.interval = truncated.interval
			newIntervalOperation.through = truncated.Through()
			// Added back to the pending because additional curation could be
			// necessary.
			out = append(out, &newIntervalOperation)
		}
	} else {
		// Operation is OK as-is.
		out = append(out, group[0])
	}

	return
}

// Flattens all groups of time according to greed.
func optimizeTimeGroups(pending ops) (out ops) {
	if len(pending) == 0 {
		return
	}

	sort.Sort(pending)

	nextOperation := pending[0]
	groupedQueries := selectQueriesForTime(nextOperation.StartsAt(), pending)
	out = optimizeTimeGroup(groupedQueries)
	pending = pending[len(groupedQueries):len(pending)]

	tail := optimizeTimeGroups(pending)

	return append(out, tail...)
}

func optimize(pending ops) (out ops) {
	return optimizeForward(optimizeTimeGroups(pending))
}
