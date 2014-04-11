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

package tiered

import (
	"container/heap"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

// viewRequestBuilder contains the various requests for data.
type viewRequestBuilder struct {
	storage    *TieredStorage
	operations ops
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

// Execute implements ViewRequestBuilder.
func (v *viewRequestBuilder) Execute(deadline time.Duration, queryStats *stats.TimerGroup) (metric.View, error) {
	return v.storage.makeView(v, deadline, queryStats)
}

// PopOp implements ViewRequestBuilder.
func (v *viewRequestBuilder) PopOp() metric.Op {
	return heap.Pop(&v.operations).(metric.Op)
}

// HasOp implements ViewRequestBuilder.
func (v *viewRequestBuilder) HasOp() bool {
	return v.operations.Len() > 0
}

type view struct {
	*memorySeriesStorage
}

func (v view) appendSamples(fingerprint *clientmodel.Fingerprint, samples metric.Values) {
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
