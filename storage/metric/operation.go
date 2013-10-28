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
	"math"
	"sort"

	clientmodel "github.com/prometheus/client_golang/model"
)

// Encapsulates a primitive query operation.
type op interface {
	// The time at which this operation starts.
	StartsAt() clientmodel.Timestamp
	// Extract samples from stream of values and advance operation time.
	ExtractSamples(Values) Values
	// Return whether the operator has consumed all data it needs.
	Consumed() bool
	// Get current operation time or nil if no subsequent work associated with
	// this operator remains.
	CurrentTime() clientmodel.Timestamp
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
	time     clientmodel.Timestamp
	consumed bool
}

func (g *getValuesAtTimeOp) String() string {
	return fmt.Sprintf("getValuesAtTimeOp at %s", g.time)
}

func (g *getValuesAtTimeOp) StartsAt() clientmodel.Timestamp {
	return g.time
}

func (g *getValuesAtTimeOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	out = extractValuesAroundTime(g.time, in)
	g.consumed = true
	return
}

func (g *getValuesAtTimeOp) GreedierThan(op op) (superior bool) {
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
func extractValuesAroundTime(t clientmodel.Timestamp, in Values) (out Values) {
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
			out = in[i : i+1]
		} else {
			if i == 0 {
				// We hit before the first sample time. Return only the first sample.
				out = in[0:1]
			} else {
				// We hit between two samples. Return both surrounding samples.
				out = in[i-1 : i+1]
			}
		}
	}
	return
}

func (g getValuesAtTimeOp) CurrentTime() clientmodel.Timestamp {
	return g.time
}

func (g getValuesAtTimeOp) Consumed() bool {
	return g.consumed
}

// Encapsulates getting values at a given interval over a duration.
type getValuesAtIntervalOp struct {
	from     clientmodel.Timestamp
	through  clientmodel.Timestamp
	interval clientmodel.Duration
}

func (o *getValuesAtIntervalOp) String() string {
	return fmt.Sprintf("getValuesAtIntervalOp from %s each %s through %s", o.from, o.interval, o.through)
}

func (g *getValuesAtIntervalOp) StartsAt() clientmodel.Timestamp {
	return g.from
}

func (g *getValuesAtIntervalOp) Through() clientmodel.Timestamp {
	return g.through
}

func (g *getValuesAtIntervalOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	lastChunkTime := in[len(in)-1].Timestamp
	for len(in) > 0 {
		out = append(out, extractValuesAroundTime(g.from, in)...)
		lastExtractedTime := out[len(out)-1].Timestamp
		in = in.TruncateBefore(lastExtractedTime.Add(1))
		g.from = g.from.Add(g.interval)
		for !g.from.After(lastExtractedTime) {
			g.from = g.from.Add(g.interval)
		}
		if lastExtractedTime.Equal(lastChunkTime) {
			break
		}
		if g.from.After(g.through) {
			break
		}
	}
	return
}

func (g *getValuesAtIntervalOp) CurrentTime() clientmodel.Timestamp {
	return g.from
}

func (g *getValuesAtIntervalOp) Consumed() bool {
	return g.from.After(g.through)
}

func (g *getValuesAtIntervalOp) GreedierThan(op op) (superior bool) {
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

// Encapsulates getting all values in a given range.
type getValuesAlongRangeOp struct {
	from    clientmodel.Timestamp
	through clientmodel.Timestamp
}

func (o *getValuesAlongRangeOp) String() string {
	return fmt.Sprintf("getValuesAlongRangeOp from %s through %s", o.from, o.through)
}

func (g *getValuesAlongRangeOp) StartsAt() clientmodel.Timestamp {
	return g.from
}

func (g *getValuesAlongRangeOp) Through() clientmodel.Timestamp {
	return g.through
}

func (g *getValuesAlongRangeOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	// Find the first sample where time >= g.from.
	firstIdx := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(g.from)
	})
	if firstIdx == len(in) {
		// No samples at or after operator start time. This can only happen if we
		// try applying the operator to a time after the last recorded sample. In
		// this case, we're finished.
		g.from = g.through.Add(1)
		return
	}

	// Find the first sample where time > g.through.
	lastIdx := sort.Search(len(in), func(i int) bool {
		return in[i].Timestamp.After(g.through)
	})
	if lastIdx == firstIdx {
		g.from = g.through.Add(1)
		return
	}

	lastSampleTime := in[lastIdx-1].Timestamp
	// Sample times are stored with a maximum time resolution of one second, so
	// we have to add exactly that to target the next chunk on the next op
	// iteration.
	g.from = lastSampleTime.Add(clientmodel.Second)
	return in[firstIdx:lastIdx]
}

func (g *getValuesAlongRangeOp) CurrentTime() clientmodel.Timestamp {
	return g.from
}

func (g *getValuesAlongRangeOp) Consumed() bool {
	return g.from.After(g.through)
}

func (g *getValuesAlongRangeOp) GreedierThan(op op) (superior bool) {
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

// Encapsulates getting all values from ranges along intervals.
//
// Works just like getValuesAlongRangeOp, but when from > through, through is
// incremented by interval and from is reset to through-rangeDuration. Returns
// current time nil when from > totalThrough.
type getValueRangeAtIntervalOp struct {
	rangeFrom     clientmodel.Timestamp
	rangeThrough  clientmodel.Timestamp
	rangeDuration clientmodel.Duration
	interval      clientmodel.Duration
	through       clientmodel.Timestamp
}

func (o *getValueRangeAtIntervalOp) String() string {
	return fmt.Sprintf("getValueRangeAtIntervalOp range %s from %s each %s through %s", o.rangeDuration, o.rangeFrom, o.interval, o.through)
}

func (g *getValueRangeAtIntervalOp) StartsAt() clientmodel.Timestamp {
	return g.rangeFrom
}

func (g *getValueRangeAtIntervalOp) Through() clientmodel.Timestamp {
	panic("not implemented")
}

func (g *getValueRangeAtIntervalOp) advanceToNextInterval() {
	g.rangeThrough = g.rangeThrough.Add(g.interval)
	g.rangeFrom = g.rangeThrough.Add(-g.rangeDuration)
}

func (g *getValueRangeAtIntervalOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	// Find the first sample where time >= g.from.
	firstIdx := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(g.rangeFrom)
	})
	if firstIdx == len(in) {
		// No samples at or after operator start time. This can only happen if we
		// try applying the operator to a time after the last recorded sample. In
		// this case, we're finished.
		g.rangeFrom = g.through.Add(1)
		return
	}

	// Find the first sample where time > g.rangeThrough.
	lastIdx := sort.Search(len(in), func(i int) bool {
		return in[i].Timestamp.After(g.rangeThrough)
	})
	// This only happens when there is only one sample and it is both after
	// g.rangeFrom and after g.rangeThrough. In this case, both indexes are 0.
	if lastIdx == firstIdx {
		g.advanceToNextInterval()
		return
	}

	lastSampleTime := in[lastIdx-1].Timestamp
	// Sample times are stored with a maximum time resolution of one second, so
	// we have to add exactly that to target the next chunk on the next op
	// iteration.
	g.rangeFrom = lastSampleTime.Add(clientmodel.Second)
	if g.rangeFrom.After(g.rangeThrough) {
		g.advanceToNextInterval()
	}
	return in[firstIdx:lastIdx]
}

func (g *getValueRangeAtIntervalOp) CurrentTime() clientmodel.Timestamp {
	return g.rangeFrom
}

func (g *getValueRangeAtIntervalOp) Consumed() bool {
	return g.rangeFrom.After(g.through)
}

func (g *getValueRangeAtIntervalOp) GreedierThan(op op) bool {
	panic("not implemented")
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

	Through() clientmodel.Timestamp
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
func collectIntervals(o ops) (intervals map[clientmodel.Duration]ops) {
	intervals = make(map[clientmodel.Duration]ops)

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
func optimizeForward(unoptimized ops) ops {
	if len(unoptimized) <= 1 {
		return unoptimized
	}

	head := unoptimized[0]
	unoptimized = unoptimized[1:]
	optimized := ops{}

	switch headOp := head.(type) {
	case *getValuesAtTimeOp:
		optimized = ops{head}
	case *getValuesAtIntervalOp:
		optimized, unoptimized = optimizeForwardGetValuesAtInterval(headOp, unoptimized)
	case *getValuesAlongRangeOp:
		optimized, unoptimized = optimizeForwardGetValuesAlongRange(headOp, unoptimized)
	default:
		panic("unknown operation type")
	}

	tail := optimizeForward(unoptimized)

	return append(optimized, tail...)
}

func optimizeForwardGetValuesAtInterval(headOp *getValuesAtIntervalOp, unoptimized ops) (ops, ops) {
	// If the last value was a scan at a given frequency along an interval,
	// several optimizations may exist.
	for _, peekOperation := range unoptimized {
		if peekOperation.StartsAt().After(headOp.Through()) {
			break
		}

		// If the type is not a range request, we can't do anything.
		switch next := peekOperation.(type) {
		case *getValuesAlongRangeOp:
			if !next.GreedierThan(headOp) {
				after := getValuesAtIntervalOp(*headOp)

				headOp.through = next.from

				// Truncate the get value at interval request if a range request cuts
				// it off somewhere.
				from := next.from

				for !from.After(next.through) {
					from = from.Add(headOp.interval)
				}

				after.from = from

				unoptimized = append(ops{&after}, unoptimized...)
				sort.Sort(startsAtSort{unoptimized})
				return ops{headOp}, unoptimized
			}
		}
	}
	return ops{headOp}, unoptimized
}

func optimizeForwardGetValuesAlongRange(headOp *getValuesAlongRangeOp, unoptimized ops) (ops, ops) {
	optimized := ops{}
	for _, peekOperation := range unoptimized {
		if peekOperation.StartsAt().After(headOp.Through()) {
			optimized = ops{headOp}
			break
		}

		switch next := peekOperation.(type) {
		// All values at a specific time may be elided into the range query.
		case *getValuesAtTimeOp:
			optimized = ops{headOp}
			unoptimized = unoptimized[1:]
		case *getValuesAlongRangeOp:
			// Range queries should be concatenated if they overlap.
			if next.GreedierThan(headOp) {
				next.from = headOp.from
				return optimized, unoptimized
			}
			optimized = ops{headOp}
			unoptimized = unoptimized[1:]
		case *getValuesAtIntervalOp:
			optimized = ops{headOp}
			unoptimized = unoptimized[1:]

			if next.GreedierThan(headOp) {
				nextStart := next.from

				for !nextStart.After(headOp.through) {
					nextStart = nextStart.Add(next.interval)
				}

				next.from = nextStart
				unoptimized = append(ops{next}, unoptimized...)
			}
		default:
			panic("unknown operation type")
		}
	}
	return optimized, unoptimized
}

// selectQueriesForTime chooses all subsequent operations from the slice that
// have the same start time as the provided time and emits them.
func selectQueriesForTime(time clientmodel.Timestamp, queries ops) (out ops) {
	if len(queries) == 0 {
		return
	}

	if !queries[0].StartsAt().Equal(time) {
		return
	}

	out = append(out, queries[0])
	tail := selectQueriesForTime(time, queries[1:])

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
func selectGreediestIntervals(in map[clientmodel.Duration]ops) (out map[clientmodel.Duration]durationOperator) {
	if len(in) == 0 {
		return
	}

	out = make(map[clientmodel.Duration]durationOperator)

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
func rewriteForGreediestInterval(greediestIntervals map[clientmodel.Duration]durationOperator) ops {
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

// rewriteForRangeAndInterval examines the existence of a range operation and a
// set of interval operations that start at the same time and deletes all
// interval operations that start and finish before the range operation
// completes and rewrites all interval operations that continue longer than
// the range operation to start at the next best increment after the range.
//
// Assume that we have a range operator O1 and two interval operations O2 and
// O3.  O2 and O3 have the same period (i.e., sampling interval), but O2
// terminates before O1 and O3 continue beyond O1.
//
// O1------------>|
// T1------------T7
//
// O2-->|-->|-->|
// T1----------T6
//
// O3-->|-->|-->|-->|-->|
// T1------------------T10
//
// This scenario will be rewritten such that O2 is deleted and O3 is truncated
// from T1 through T7, and O3's new starting time is at T7 and runs through T10:
//
// O1------------>|
// T1------------T7
//
//               O2>|-->|
//               T7---T10
//
// All rewritten interval operators will respect their original start time
// multipliers.
func rewriteForRangeAndInterval(greediestRange durationOperator, greediestIntervals map[clientmodel.Duration]durationOperator) (out ops) {
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
			remainingSlice    = greediestRange.Through().Sub(greediestRange.StartsAt()) / clientmodel.Second
			nextIntervalPoint = clientmodel.Duration(math.Ceil(float64(remainingSlice)/float64(truncated.interval)) * float64(truncated.interval/clientmodel.Second))
			nextStart         = greediestRange.Through().Add(nextIntervalPoint)
		)

		newIntervalOperation.from = nextStart
		newIntervalOperation.interval = truncated.interval
		newIntervalOperation.through = truncated.Through()
		// Added back to the pending because additional curation could be
		// necessary.
		out = append(out, &newIntervalOperation)
	}

	return
}

// Flattens queries that occur at the same time according to duration and level
// of greed.  Consult the various rewriter functions for their respective modes
// of operation.
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
		out = rewriteForRangeAndInterval(greediestRange, greediestIntervals)
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

	var (
		nextOperation  = pending[0]
		groupedQueries = selectQueriesForTime(nextOperation.StartsAt(), pending)
	)

	out = optimizeTimeGroup(groupedQueries)
	pending = pending[len(groupedQueries):]

	tail := optimizeTimeGroups(pending)

	return append(out, tail...)
}

func optimize(pending ops) (out ops) {
	return optimizeForward(optimizeTimeGroups(pending))
}
