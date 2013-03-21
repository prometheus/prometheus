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
	"github.com/prometheus/prometheus/model"
	"github.com/ryszard/goskiplist/skiplist"
	"sort"
	"time"
)

var (
	// firstSupertime is the smallest valid supertime that may be seeked to.
	firstSupertime = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	// lastSupertime is the largest valid supertime that may be seeked to.
	lastSupertime = []byte{127, 255, 255, 255, 255, 255, 255, 255}
)

// Represents the summation of all datastore queries that shall be performed to
// extract values.  Each operation mutates the state of the builder.
type ViewRequestBuilder interface {
	GetMetricAtTime(fingerprint model.Fingerprint, time time.Time)
	GetMetricAtInterval(fingerprint model.Fingerprint, from, through time.Time, interval time.Duration)
	GetMetricRange(fingerprint model.Fingerprint, from, through time.Time)
	ScanJobs() scanJobs
}

// Contains the various unoptimized requests for data.
type viewRequestBuilder struct {
	operations map[model.Fingerprint]ops
}

// Furnishes a ViewRequestBuilder for remarking what types of queries to perform.
func NewViewRequestBuilder() viewRequestBuilder {
	return viewRequestBuilder{
		operations: make(map[model.Fingerprint]ops),
	}
}

// Gets for the given Fingerprint either the value at that time if there is an
// match or the one or two values adjacent thereto.
func (v viewRequestBuilder) GetMetricAtTime(fingerprint model.Fingerprint, time time.Time) {
	ops := v.operations[fingerprint]
	ops = append(ops, &getValuesAtTimeOp{
		time: time,
	})
	v.operations[fingerprint] = ops
}

// Gets for the given Fingerprint either the value at that interval from From
// through Through  if there is an match or the one or two values adjacent
// for each point.
func (v viewRequestBuilder) GetMetricAtInterval(fingerprint model.Fingerprint, from, through time.Time, interval time.Duration) {
	ops := v.operations[fingerprint]
	ops = append(ops, &getValuesAtIntervalOp{
		from:     from,
		through:  through,
		interval: interval,
	})
	v.operations[fingerprint] = ops
}

// Gets for the given Fingerprint either the values that occur inclusively from
// From through Through.
func (v viewRequestBuilder) GetMetricRange(fingerprint model.Fingerprint, from, through time.Time) {
	ops := v.operations[fingerprint]
	ops = append(ops, &getValuesAlongRangeOp{
		from:    from,
		through: through,
	})
	v.operations[fingerprint] = ops
}

// Emits the optimized scans that will occur in the data store.  This
// effectively resets the ViewRequestBuilder back to a pristine state.
func (v viewRequestBuilder) ScanJobs() (j scanJobs) {
	for fingerprint, operations := range v.operations {
		sort.Sort(startsAtSort{operations})

		j = append(j, scanJob{
			fingerprint: fingerprint,
			operations:  optimize(operations),
		})

		delete(v.operations, fingerprint)
	}

	sort.Sort(j)

	return
}

type view struct {
	fingerprintToSeries map[model.Fingerprint]viewStream
}

func (v view) appendSample(fingerprint model.Fingerprint, timestamp time.Time, value model.SampleValue) {
	var (
		series, ok = v.fingerprintToSeries[fingerprint]
	)

	if !ok {
		series = newViewStream()
		v.fingerprintToSeries[fingerprint] = series
	}

	series.add(timestamp, value)
}

func (v view) Close() {
	v.fingerprintToSeries = make(map[model.Fingerprint]viewStream)
}

func (v view) GetValueAtTime(f model.Fingerprint, t time.Time) (samples []model.SamplePair) {
	series, ok := v.fingerprintToSeries[f]
	if !ok {
		return
	}

	iterator := series.values.Seek(skipListTime(t))
	if iterator == nil {
		// If the iterator is nil, it means we seeked past the end of the series,
		// so we seek to the last value instead. Due to the reverse ordering
		// defined on skipListTime, this corresponds to the sample with the
		// earliest timestamp.
		iterator = series.values.SeekToLast()
		if iterator == nil {
			// The list is empty.
			return
		}
	}

	defer iterator.Close()

	if iterator.Key() == nil || iterator.Value() == nil {
		return
	}

	samples = append(samples, model.SamplePair{
		Timestamp: time.Time(iterator.Key().(skipListTime)),
		Value:     iterator.Value().(value).get(),
	})

	if iterator.Previous() {
		samples = append(samples, model.SamplePair{
			Timestamp: time.Time(iterator.Key().(skipListTime)),
			Value:     iterator.Value().(value).get(),
		})
	}

	return
}

func (v view) GetBoundaryValues(f model.Fingerprint, i model.Interval) (first []model.SamplePair, second []model.SamplePair) {
	first = v.GetValueAtTime(f, i.OldestInclusive)
	second = v.GetValueAtTime(f, i.NewestInclusive)
	return
}

func (v view) GetRangeValues(f model.Fingerprint, i model.Interval) (samples []model.SamplePair) {
	series, ok := v.fingerprintToSeries[f]
	if !ok {
		return
	}

	iterator := series.values.Seek(skipListTime(i.OldestInclusive))
	if iterator == nil {
		// If the iterator is nil, it means we seeked past the end of the series,
		// so we seek to the last value instead. Due to the reverse ordering
		// defined on skipListTime, this corresponds to the sample with the
		// earliest timestamp.
		iterator = series.values.SeekToLast()
		if iterator == nil {
			// The list is empty.
			return
		}
	}

	for {
		timestamp := time.Time(iterator.Key().(skipListTime))
		if timestamp.After(i.NewestInclusive) {
			break
		}

		if !timestamp.Before(i.OldestInclusive) {
			samples = append(samples, model.SamplePair{
				Value:     iterator.Value().(value).get(),
				Timestamp: timestamp,
			})
		}

		if !iterator.Previous() {
			break
		}
	}

	return
}

func newView() view {
	return view{
		fingerprintToSeries: make(map[model.Fingerprint]viewStream),
	}
}

type viewStream struct {
	values *skiplist.SkipList
}

func (s viewStream) add(timestamp time.Time, value model.SampleValue) {
	s.values.Set(skipListTime(timestamp), singletonValue(value))
}

func newViewStream() viewStream {
	return viewStream{
		values: skiplist.New(),
	}
}
