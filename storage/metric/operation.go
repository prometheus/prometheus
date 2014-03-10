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
	"sort"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

// op encapsulates a primitive query operation.
type op interface {
	// Fingerprint returns the fingerprint of the metric this operation
	// operates on.
	Fingerprint() *clientmodel.Fingerprint
	// ExtractSamples extracts samples from a stream of values and advances
	// the operation time.
	ExtractSamples(Values) Values
	// Consumed returns whether the operator has consumed all data it needs.
	Consumed() bool
	// CurrentTime gets the current operation time. In a newly created op,
	// this is the starting time of the operation. During ongoing execution
	// of the op, the current time is advanced accordingly. Once no
	// subsequent work associated with the operation remains, nil is
	// returned.
	CurrentTime() clientmodel.Timestamp
}

// durationOperator encapsulates a general operation that occurs over a
// duration.
type durationOperator interface {
	op
	Through() clientmodel.Timestamp
}

// ops is a heap of operations, primary sorting key is the fingerprint.
type ops []op

// Len implements sort.Interface and heap.Interface.
func (o ops) Len() int {
	return len(o)
}

// Less implements sort.Interface and heap.Interface. It compares the
// fingerprints. If they are equal, the comparison is delegated to
// currentTimeSort.
func (o ops) Less(i, j int) bool {
	fpi := o[i].Fingerprint()
	fpj := o[j].Fingerprint()
	if fpi.Equal(fpj) {
		return currentTimeSort{o}.Less(i, j)
	}
	return fpi.Less(fpj)
}

// Swap implements sort.Interface and heap.Interface.
func (o ops) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// Push implements heap.Interface.
func (o *ops) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's
	// length, not just its contents.
	*o = append(*o, x.(op))
}

// Push implements heap.Interface.
func (o *ops) Pop() interface{} {
	old := *o
	n := len(old)
	x := old[n-1]
	*o = old[0 : n-1]
	return x
}

// currentTimeSort is a wrapper for ops with customized sorting order.
type currentTimeSort struct {
	ops
}

// currentTimeSort implements sort.Interface and sorts the operations in
// chronological order by their current time.
func (s currentTimeSort) Less(i, j int) bool {
	return s.ops[i].CurrentTime().Before(s.ops[j].CurrentTime())
}

// baseOp contains the implementations and fields shared between different op
// types.
type baseOp struct {
	fp      clientmodel.Fingerprint
	current clientmodel.Timestamp
}

func (g *baseOp) Fingerprint() *clientmodel.Fingerprint {
	return &g.fp
}

func (g *baseOp) CurrentTime() clientmodel.Timestamp {
	return g.current
}

// getValuesAtTimeOp encapsulates getting values at or adjacent to a specific
// time.
type getValuesAtTimeOp struct {
	baseOp
	consumed bool
}

func (g *getValuesAtTimeOp) String() string {
	return fmt.Sprintf("getValuesAtTimeOp at %s", g.current)
}

func (g *getValuesAtTimeOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	out = extractValuesAroundTime(g.current, in)
	g.consumed = true
	return
}

func (g getValuesAtTimeOp) Consumed() bool {
	return g.consumed
}

// getValuesAlongRangeOp encapsulates getting all values in a given range.
type getValuesAlongRangeOp struct {
	baseOp
	through clientmodel.Timestamp
}

func (g *getValuesAlongRangeOp) String() string {
	return fmt.Sprintf("getValuesAlongRangeOp from %s through %s", g.current, g.through)
}

func (g *getValuesAlongRangeOp) Through() clientmodel.Timestamp {
	return g.through
}

func (g *getValuesAlongRangeOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	// Find the first sample where time >= g.current.
	firstIdx := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(g.current)
	})
	if firstIdx == len(in) {
		// No samples at or after operator start time. This can only
		// happen if we try applying the operator to a time after the
		// last recorded sample. In this case, we're finished.
		g.current = g.through.Add(clientmodel.MinimumTick)
		return
	}

	// Find the first sample where time > g.through.
	lastIdx := sort.Search(len(in), func(i int) bool {
		return in[i].Timestamp.After(g.through)
	})
	if lastIdx == firstIdx {
		g.current = g.through.Add(clientmodel.MinimumTick)
		return
	}

	lastSampleTime := in[lastIdx-1].Timestamp
	// Sample times are stored with a maximum time resolution of one second,
	// so we have to add exactly that to target the next chunk on the next
	// op iteration.
	g.current = lastSampleTime.Add(time.Second)
	return in[firstIdx:lastIdx]
}

func (g *getValuesAlongRangeOp) Consumed() bool {
	return g.current.After(g.through)
}

// getValuesAtIntervalOp encapsulates getting values at a given interval over a
// duration.
type getValuesAtIntervalOp struct {
	getValuesAlongRangeOp
	interval time.Duration
}

func (g *getValuesAtIntervalOp) String() string {
	return fmt.Sprintf("getValuesAtIntervalOp from %s each %s through %s", g.current, g.interval, g.through)
}

func (g *getValuesAtIntervalOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	lastChunkTime := in[len(in)-1].Timestamp
	for len(in) > 0 {
		out = append(out, extractValuesAroundTime(g.current, in)...)
		lastExtractedTime := out[len(out)-1].Timestamp
		in = in.TruncateBefore(lastExtractedTime.Add(
			clientmodel.MinimumTick))
		g.current = g.current.Add(g.interval)
		for !g.current.After(lastExtractedTime) {
			g.current = g.current.Add(g.interval)
		}
		if lastExtractedTime.Equal(lastChunkTime) {
			break
		}
		if g.current.After(g.through) {
			break
		}
	}
	return
}

// getValueRangeAtIntervalOp encapsulates getting all values from ranges along
// intervals.
//
// Works just like getValuesAlongRangeOp, but when from > through, through is
// incremented by interval and from is reset to through-rangeDuration. Returns
// current time nil when from > totalThrough.
type getValueRangeAtIntervalOp struct {
	getValuesAtIntervalOp
	rangeThrough  clientmodel.Timestamp
	rangeDuration time.Duration
}

func (g *getValueRangeAtIntervalOp) String() string {
	return fmt.Sprintf("getValueRangeAtIntervalOp range %s from %s each %s through %s", g.rangeDuration, g.current, g.interval, g.through)
}

// Through panics because the notion of 'through' is ambiguous for this op.
func (g *getValueRangeAtIntervalOp) Through() clientmodel.Timestamp {
	panic("not implemented")
}

func (g *getValueRangeAtIntervalOp) advanceToNextInterval() {
	g.rangeThrough = g.rangeThrough.Add(g.interval)
	g.current = g.rangeThrough.Add(-g.rangeDuration)
}

func (g *getValueRangeAtIntervalOp) ExtractSamples(in Values) (out Values) {
	if len(in) == 0 {
		return
	}
	// Find the first sample where time >= g.current.
	firstIdx := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(g.current)
	})
	if firstIdx == len(in) {
		// No samples at or after operator start time. This can only
		// happen if we try applying the operator to a time after the
		// last recorded sample. In this case, we're finished.
		g.current = g.through.Add(clientmodel.MinimumTick)
		return
	}

	// Find the first sample where time > g.rangeThrough.
	lastIdx := sort.Search(len(in), func(i int) bool {
		return in[i].Timestamp.After(g.rangeThrough)
	})
	// This only happens when there is only one sample and it is both after
	// g.current and after g.rangeThrough. In this case, both indexes are 0.
	if lastIdx == firstIdx {
		g.advanceToNextInterval()
		return
	}

	lastSampleTime := in[lastIdx-1].Timestamp
	// Sample times are stored with a maximum time resolution of one second,
	// so we have to add exactly that to target the next chunk on the next
	// op iteration.
	g.current = lastSampleTime.Add(time.Second)
	if g.current.After(g.rangeThrough) {
		g.advanceToNextInterval()
	}
	return in[firstIdx:lastIdx]
}

// getValuesAtIntervalOps contains getValuesAtIntervalOp operations. It
// implements sort.Interface and sorts the operations in ascending order by
// their frequency.
type getValuesAtIntervalOps []*getValuesAtIntervalOp

func (s getValuesAtIntervalOps) Len() int {
	return len(s)
}

func (s getValuesAtIntervalOps) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s getValuesAtIntervalOps) Less(i, j int) bool {
	return s[i].interval < s[j].interval
}

// extractValuesAroundTime searches for the provided time in the list of
// available samples and emits a slice containing the data points that
// are adjacent to it.
//
// An assumption of this is that the provided samples are already sorted!
func extractValuesAroundTime(t clientmodel.Timestamp, in Values) Values {
	i := sort.Search(len(in), func(i int) bool {
		return !in[i].Timestamp.Before(t)
	})
	if i == len(in) {
		// Target time is past the end, return only the last sample.
		return in[len(in)-1:]
	}
	if in[i].Timestamp.Equal(t) && len(in) > i+1 {
		// We hit exactly the current sample time. Very unlikely in
		// practice.  Return only the current sample.
		return in[i : i+1]
	}
	if i == 0 {
		// We hit before the first sample time. Return only the first
		// sample.
		return in[0:1]
	}
	// We hit between two samples. Return both surrounding samples.
	return in[i-1 : i+1]
}
