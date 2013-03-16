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
	// GreedierThan indicates whether this present operation should take
	// precedence over the other operation due to greediness.
	//
	// A critical assumption is that this operator and the other occur at the
	// same time: this.StartsAt().Equal(op.StartsAt()).
	GreedierThan(op) bool
}

// Provides a sortable collection of operations.
type ops []op

func (o ops) Len() int {
	return len(o)
}

// startsAtSort implements the sorting protocol and allows operator to be sorted
// in chronological order by when they start.
type startsAtSort struct {
	ops
}

func (s startsAtSort) Less(i, j int) bool {
	return s.ops[i].StartsAt().Before(s.ops[j].StartsAt())
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

func (g getValuesAtTimeOp) GreedierThan(op op) (superior bool) {
	switch op.(type) {
	case *getValuesAtTimeOp:
		superior = true
	case durationOperator:
		superior = false
	default:
		panic("unknown operation")
	}

	return
}

// extractValuesAroundTime searches for the provided time in the list of
// available samples and emits a slice containing the data points that
// are adjacent to it.
//
// An assumption of this is that the provided samples are already sorted!
func extractValuesAroundTime(t time.Time, in []model.SamplePair) (out []model.SamplePair) {
	i := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(t)
	})
	if i == len(in) {
		// Target time is past the end, return only the last sample.
		out = in[len(in)-1:]
	} else {
		if in[i].Timestamp.Equal(t) && len(in) > i+1 {
			// We hit exactly the current sample time. Very unlikely in practice.
			// Return only the current sample.
			out = append(out, in[i])
		} else {
			if i == 0 {
				// We hit before the first sample time. Return only the first sample.
				out = append(out, in[0:1]...)
			} else {
				// We hit between two samples. Return both surrounding samples.
				out = append(out, in[i-1:i+1]...)
			}
		}
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

func (g getValuesAtIntervalOp) GreedierThan(op op) (superior bool) {
	switch o := op.(type) {
	case *getValuesAtTimeOp:
		superior = true
	case durationOperator:
		superior = g.Through().After(o.Through())
	default:
		panic("unknown operation")
	}

	return
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

func (g getValuesAlongRangeOp) GreedierThan(op op) (superior bool) {
	switch o := op.(type) {
	case *getValuesAtTimeOp:
		superior = true
	case durationOperator:
		superior = g.Through().After(o.Through())
	default:
		panic("unknown operation")
	}

	return
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

// greedinessSort sorts the operations in descending order by level of
// greediness.
type greedinessSort struct {
	ops
}

func (g greedinessSort) Less(i, j int) bool {
	return g.ops[i].GreedierThan(g.ops[j])
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

// Selects and returns all operations that are getValuesAtIntervalOp operations
// in a map whereby the operation interval is the key and the value are the
// operations sorted by respective level of greediness.
func collectIntervals(o ops) (intervals map[time.Duration]ops) {
	intervals = make(map[time.Duration]ops)

	for _, operation := range o {
		switch t := operation.(type) {
		case *getValuesAtIntervalOp:
			operations, _ := intervals[t.interval]

			operations = append(operations, t)
			intervals[t.interval] = operations
		}
	}

	return
}

// Selects and returns all operations that are getValuesAlongRangeOp operations.
func collectRanges(ops ops) (ranges ops) {
	for _, operation := range ops {
		switch t := operation.(type) {
		case *getValuesAlongRangeOp:
			ranges = append(ranges, t)
		}
	}

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
				if !next.GreedierThan(t) {
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
					sort.Sort(startsAtSort{pending})

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

				if next.GreedierThan(t) {
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

				if next.GreedierThan(t) {
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
	sort.Sort(startsAtSort{pending})

	tail := optimizeForward(pending)

	return append(ops{firstOperation}, tail...)
}

// selectQueriesForTime chooses all subsequent operations from the slice that
// have the same start time as the provided time and emits them.
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

// selectGreediestRange scans through the various getValuesAlongRangeOp
// operations and emits the one that is the greediest.
func selectGreediestRange(in ops) (o durationOperator) {
	if len(in) == 0 {
		return
	}

	sort.Sort(greedinessSort{in})

	o = in[0].(*getValuesAlongRangeOp)

	return
}

// selectGreediestIntervals scans through the various getValuesAtIntervalOp
// operations and emits a map of the greediest operation keyed by its start
// time.
func selectGreediestIntervals(in map[time.Duration]ops) (out map[time.Duration]durationOperator) {
	if len(in) == 0 {
		return
	}

	out = make(map[time.Duration]durationOperator)

	for i, ops := range in {
		sort.Sort(greedinessSort{ops})

		out[i] = ops[0].(*getValuesAtIntervalOp)
	}

	return
}

// rewriteForGreediestRange rewrites the current pending operation such that the
// greediest range operation takes precedence over all other operators in this
// time group.
//
// Between two range operations O1 and O2, they both start at the same time;
// however, O2 extends for a longer duration than O1.  Thusly, O1 should be
// deleted with O2.
//
// O1------>|
// T1      T4
//
// O2------------>|
// T1            T7
//
// Thusly O1 can be squashed into O2 without having side-effects.
func rewriteForGreediestRange(greediestRange durationOperator) ops {
	return ops{greediestRange}
}

// rewriteForGreediestInterval rewrites teh current pending interval operations
// such that the interval operation with the smallest collection period is
// invoked first, for it will skip around the soonest of any of the remaining
// other operators.
//
// Between two interval operations O1 and O2, they both start at the same time;
// however, O2's period is shorter than O1, meaning it will sample far more
// frequently from the underlying time series.  Thusly, O2 should start before
// O1.
//
// O1---->|---->|
// T1          T5
//
// O2->|->|->|->|
// T1          T5
//
// The rewriter presently does not scan and compact for common divisors in the
// periods, though this may be nice to have.  For instance, if O1 has a period
// of 2 and O2 has a period of 4, O2 would be dropped for O1 would implicitly
// cover its period.
func rewriteForGreediestInterval(greediestIntervals map[time.Duration]durationOperator) ops {
	var (
		memo getValuesAtIntervalOps
		out  ops
	)

	for _, o := range greediestIntervals {
		memo = append(memo, o.(*getValuesAtIntervalOp))
	}

	sort.Sort(frequencySorter{memo})

	for _, o := range memo {
		out = append(out, o)
	}

	return out
}

// Flattens queries that occur at the same time according to duration and level
// of greed.
func optimizeTimeGroup(group ops) (out ops) {
	var (
		greediestRange     = selectGreediestRange(collectRanges(group))
		greediestIntervals = selectGreediestIntervals(collectIntervals(group))
		containsRange      = greediestRange != nil
		containsInterval   = len(greediestIntervals) > 0
	)

	switch {
	case containsRange && !containsInterval:
		out = rewriteForGreediestRange(greediestRange)
	case !containsRange && containsInterval:
		out = rewriteForGreediestInterval(greediestIntervals)
	case containsRange && containsInterval:
		out = append(out, greediestRange)
		for _, op := range greediestIntervals {
			if !op.GreedierThan(greediestRange) {
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
	default:
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

	sort.Sort(startsAtSort{pending})

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
