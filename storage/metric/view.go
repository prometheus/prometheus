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
	"container/heap"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

var (
	// firstSupertime is the smallest valid supertime that may be seeked to.
	firstSupertime = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	// lastSupertime is the largest valid supertime that may be seeked to.
	lastSupertime = []byte{127, 255, 255, 255, 255, 255, 255, 255}
)

// ViewRequestBuilder represents the summation of all datastore queries that
// shall be performed to extract values. Call the Get... methods to record the
// queries. Once done, use HasScanJobs and PopScanJob to retrieve the resulting
// scanJobs. The scanJobs are sorted by their fingerprint (and, for equal
// fingerprints, by the StartsAt timestamp of their operation).
type ViewRequestBuilder interface {
	// GetMetricAtTime records a query to get, for the given Fingerprint,
	// either the value at that time if there is a match or the one or two
	// values adjacent thereto.
	GetMetricAtTime(fingerprint *clientmodel.Fingerprint, time clientmodel.Timestamp)
	// GetMetricAtInterval records a query to get, for the given
	// Fingerprint, either the value at that interval from From through
	// Through if there is a match or the one or two values adjacent for
	// each point.
	GetMetricAtInterval(fingerprint *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval time.Duration)
	// GetMetricRange records a query to get, for the given Fingerprint, the
	// values that occur inclusively from From through Through.
	GetMetricRange(fingerprint *clientmodel.Fingerprint, from, through clientmodel.Timestamp)
	// GetMetricRangeAtInterval records a query to get value ranges at
	// intervals for the given Fingerprint:
	//
	//   |----|       |----|       |----|       |----|
	//   ^    ^            ^       ^    ^            ^
	//   |    \------------/       \----/            |
	//  from     interval       rangeDuration     through
	GetMetricRangeAtInterval(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval, rangeDuration time.Duration)
	// PopOp emits the next operation in the queue (sorted by
	// fingerprint). If called while HasOps returns false, the
	// behavior is undefined.
	PopOp() op
	// HasOp returns true if there is at least one more operation in the
	// queue.
	HasOp() bool
}

// viewRequestBuilder contains the various requests for data.
type viewRequestBuilder struct {
	operations ops
}

// NewViewRequestBuilder furnishes a ViewRequestBuilder for remarking what types
// of queries to perform.
func NewViewRequestBuilder() *viewRequestBuilder {
	return &viewRequestBuilder{
		operations: ops{},
	}
}

var getValuesAtTimes = newValueAtTimeList(10 * 1024)

// GetMetricAtTime implements ViewRequestBuilder.
func (v *viewRequestBuilder) GetMetricAtTime(fp *clientmodel.Fingerprint, time clientmodel.Timestamp) {
	heap.Push(&v.operations, getValuesAtTimes.Get(fp, time))
}

var getValuesAtIntervals = newValueAtIntervalList(10 * 1024)

// GetMetricAtInterval implements ViewRequestBuilder.
func (v *viewRequestBuilder) GetMetricAtInterval(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval time.Duration) {
	heap.Push(&v.operations, getValuesAtIntervals.Get(fp, from, through, interval))
}

var getValuesAlongRanges = newValueAlongRangeList(10 * 1024)

// GetMetricRange implements ViewRequestBuilder.
func (v *viewRequestBuilder) GetMetricRange(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp) {
	heap.Push(&v.operations, getValuesAlongRanges.Get(fp, from, through))
}

var getValuesAtIntervalAlongRanges = newValueAtIntervalAlongRangeList(10 * 1024)

// GetMetricRangeAtInterval implements ViewRequestBuilder.
func (v *viewRequestBuilder) GetMetricRangeAtInterval(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval, rangeDuration time.Duration) {
	heap.Push(&v.operations, getValuesAtIntervalAlongRanges.Get(fp, from, through, interval, rangeDuration))
}

// PopOp implements ViewRequestBuilder.
func (v *viewRequestBuilder) PopOp() op {
	return heap.Pop(&v.operations).(op)
}

// HasOp implements ViewRequestBuilder.
func (v *viewRequestBuilder) HasOp() bool {
	return v.operations.Len() > 0
}

type view struct {
	*memorySeriesStorage
}

func (v view) appendSamples(fingerprint *clientmodel.Fingerprint, samples Values) {
	v.memorySeriesStorage.appendSamplesWithoutIndexing(fingerprint, samples)
}

func newView() view {
	return view{NewMemorySeriesStorage(MemorySeriesOptions{})}
}

func giveBackOp(op interface{}) bool {
	switch v := op.(type) {
	case *getValuesAtTimeOp:
		return getValuesAtTimes.Give(v)
	case *getValuesAtIntervalOp:
		return getValuesAtIntervals.Give(v)
	case *getValuesAlongRangeOp:
		return getValuesAlongRanges.Give(v)
	case *getValueRangeAtIntervalOp:
		return getValuesAtIntervalAlongRanges.Give(v)
	default:
		panic("unrecognized operation")
	}
}
