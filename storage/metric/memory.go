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
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

// An initialSeriesArenaSize of 4*60 allows for one hour's worth of storage per
// metric without any major reallocations - assuming a sample rate of 1 / 15Hz.
const initialSeriesArenaSize = 4 * 60

type stream interface {
	add(Values)

	clone() Values
	expunge(age clientmodel.Timestamp) Values

	size() int
	clear()

	metric() clientmodel.Metric

	getValueAtTime(t clientmodel.Timestamp) Values
	getBoundaryValues(in Interval) Values
	getRangeValues(in Interval) Values
}

type arrayStream struct {
	sync.RWMutex

	m      clientmodel.Metric
	values Values
}

func (s *arrayStream) metric() clientmodel.Metric {
	return s.m
}

func (s *arrayStream) add(v Values) {
	s.Lock()
	defer s.Unlock()

	s.values = append(s.values, v...)
}

func (s *arrayStream) clone() Values {
	s.RLock()
	defer s.RUnlock()

	clone := make(Values, len(s.values))
	copy(clone, s.values)

	return clone
}

func (s *arrayStream) expunge(t clientmodel.Timestamp) Values {
	s.Lock()
	defer s.Unlock()

	finder := func(i int) bool {
		return s.values[i].Timestamp.After(t)
	}

	i := sort.Search(len(s.values), finder)
	expunged := s.values[:i]
	s.values = s.values[i:]

	return expunged
}

func (s *arrayStream) getValueAtTime(t clientmodel.Timestamp) Values {
	s.RLock()
	defer s.RUnlock()

	// BUG(all): May be avenues for simplification.
	l := len(s.values)
	switch l {
	case 0:
		return Values{}
	case 1:
		return Values{s.values[0]}
	default:
		index := sort.Search(l, func(i int) bool {
			return !s.values[i].Timestamp.Before(t)
		})

		if index == 0 {
			return Values{s.values[0]}
		}
		if index == l {
			return Values{s.values[l-1]}
		}

		if s.values[index].Timestamp.Equal(t) {
			return Values{s.values[index]}
		}
		return Values{s.values[index-1], s.values[index]}
	}
}

func (s *arrayStream) getBoundaryValues(in Interval) Values {
	s.RLock()
	defer s.RUnlock()

	oldest := sort.Search(len(s.values), func(i int) bool {
		return !s.values[i].Timestamp.Before(in.OldestInclusive)
	})

	newest := sort.Search(len(s.values), func(i int) bool {
		return s.values[i].Timestamp.After(in.NewestInclusive)
	})

	resultRange := s.values[oldest:newest]
	switch len(resultRange) {
	case 0:
		return Values{}
	case 1:
		return Values{resultRange[0]}
	default:
		return Values{resultRange[0], resultRange[len(resultRange)-1]}
	}
}

func (s *arrayStream) getRangeValues(in Interval) Values {
	s.RLock()
	defer s.RUnlock()

	oldest := sort.Search(len(s.values), func(i int) bool {
		return !s.values[i].Timestamp.Before(in.OldestInclusive)
	})

	newest := sort.Search(len(s.values), func(i int) bool {
		return s.values[i].Timestamp.After(in.NewestInclusive)
	})

	result := make(Values, newest-oldest)
	copy(result, s.values[oldest:newest])

	return result
}

func (s *arrayStream) size() int {
	return len(s.values)
}

func (s *arrayStream) clear() {
	s.values = Values{}
}

func newArrayStream(metric clientmodel.Metric) *arrayStream {
	return &arrayStream{
		m:      metric,
		values: make(Values, 0, initialSeriesArenaSize),
	}
}

type memorySeriesStorage struct {
	sync.RWMutex

	wmCache                 *watermarkCache
	fingerprintToSeries     map[clientmodel.Fingerprint]stream
	labelPairToFingerprints map[LabelPair]utility.Set
}

// MemorySeriesOptions bundles options used by NewMemorySeriesStorage to create
// a memory series storage.
type MemorySeriesOptions struct {
	// If provided, this WatermarkCache will be updated for any samples that
	// are appended to the memorySeriesStorage.
	WatermarkCache *watermarkCache
}

func (s *memorySeriesStorage) AppendSamples(samples clientmodel.Samples) error {
	for _, sample := range samples {
		s.AppendSample(sample)
	}

	return nil
}

func (s *memorySeriesStorage) AppendSample(sample *clientmodel.Sample) error {
	s.Lock()
	defer s.Unlock()

	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromMetric(sample.Metric)
	series := s.getOrCreateSeries(sample.Metric, fingerprint)
	series.add(Values{
		SamplePair{
			Value:     sample.Value,
			Timestamp: sample.Timestamp,
		},
	})

	if s.wmCache != nil {
		s.wmCache.Put(fingerprint, &watermarks{High: sample.Timestamp})
	}

	return nil
}

func (s *memorySeriesStorage) CreateEmptySeries(metric clientmodel.Metric) {
	s.Lock()
	defer s.Unlock()

	m := clientmodel.Metric{}
	for label, value := range metric {
		m[label] = value
	}

	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromMetric(m)
	s.getOrCreateSeries(m, fingerprint)
}

func (s *memorySeriesStorage) getOrCreateSeries(metric clientmodel.Metric, fingerprint *clientmodel.Fingerprint) stream {
	series, ok := s.fingerprintToSeries[*fingerprint]

	if !ok {
		series = newArrayStream(metric)
		s.fingerprintToSeries[*fingerprint] = series

		for k, v := range metric {
			labelPair := LabelPair{
				Name:  k,
				Value: v,
			}

			set, ok := s.labelPairToFingerprints[labelPair]
			if !ok {
				set = utility.Set{}
				s.labelPairToFingerprints[labelPair] = set
			}
			set.Add(*fingerprint)
		}
	}
	return series
}

func (s *memorySeriesStorage) Flush(flushOlderThan clientmodel.Timestamp, queue chan<- clientmodel.Samples) {
	emptySeries := []clientmodel.Fingerprint{}

	s.RLock()
	for fingerprint, stream := range s.fingerprintToSeries {
		toArchive := stream.expunge(flushOlderThan)
		queued := make(clientmodel.Samples, 0, len(toArchive))
		// NOTE: This duplication will go away soon.
		for _, value := range toArchive {
			queued = append(queued, &clientmodel.Sample{
				Metric:    stream.metric(),
				Timestamp: value.Timestamp,
				Value:     value.Value,
			})
		}

		// BUG(all): this can deadlock if the queue is full, as we only ever clear
		// the queue after calling this method:
		// https://github.com/prometheus/prometheus/issues/275
		if len(queued) > 0 {
			queue <- queued
		}

		if stream.size() == 0 {
			emptySeries = append(emptySeries, fingerprint)
		}
	}
	s.RUnlock()

	for _, fingerprint := range emptySeries {
		if series, ok := s.fingerprintToSeries[fingerprint]; ok && series.size() == 0 {
			s.Lock()
			s.dropSeries(&fingerprint)
			s.Unlock()
		}
	}
}

// Drop all references to a series, including any samples.
func (s *memorySeriesStorage) dropSeries(fingerprint *clientmodel.Fingerprint) {
	series, ok := s.fingerprintToSeries[*fingerprint]
	if !ok {
		return
	}

	for k, v := range series.metric() {
		labelPair := LabelPair{
			Name:  k,
			Value: v,
		}
		if set, ok := s.labelPairToFingerprints[labelPair]; ok {
			set.Remove(*fingerprint)
			if len(set) == 0 {
				delete(s.labelPairToFingerprints, labelPair)
			}
		}
	}
	delete(s.fingerprintToSeries, *fingerprint)
}

// Append raw samples, bypassing indexing. Only used to add data to views,
// which don't need to lookup by metric.
func (s *memorySeriesStorage) appendSamplesWithoutIndexing(fingerprint *clientmodel.Fingerprint, samples Values) {
	s.Lock()
	defer s.Unlock()

	series, ok := s.fingerprintToSeries[*fingerprint]

	if !ok {
		series = newArrayStream(clientmodel.Metric{})
		s.fingerprintToSeries[*fingerprint] = series
	}

	series.add(samples)
}

func (s *memorySeriesStorage) GetFingerprintsForLabelSet(l clientmodel.LabelSet) (clientmodel.Fingerprints, error) {
	s.RLock()
	defer s.RUnlock()

	sets := []utility.Set{}
	for k, v := range l {
		set, ok := s.labelPairToFingerprints[LabelPair{
			Name:  k,
			Value: v,
		}]
		if !ok {
			return nil, nil
		}
		sets = append(sets, set)
	}

	setCount := len(sets)
	if setCount == 0 {
		return nil, nil
	}

	base := sets[0]
	for i := 1; i < setCount; i++ {
		base = base.Intersection(sets[i])
	}

	fingerprints := clientmodel.Fingerprints{}
	for _, e := range base.Elements() {
		fingerprint := e.(clientmodel.Fingerprint)
		fingerprints = append(fingerprints, &fingerprint)
	}

	return fingerprints, nil
}

func (s *memorySeriesStorage) GetMetricForFingerprint(f *clientmodel.Fingerprint) (clientmodel.Metric, error) {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil, nil
	}

	metric := clientmodel.Metric{}
	for label, value := range series.metric() {
		metric[label] = value
	}

	return metric, nil
}

func (s *memorySeriesStorage) HasFingerprint(f *clientmodel.Fingerprint) bool {
	s.RLock()
	defer s.RUnlock()

	_, has := s.fingerprintToSeries[*f]

	return has
}

func (s *memorySeriesStorage) CloneSamples(f *clientmodel.Fingerprint) Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.clone()
}

func (s *memorySeriesStorage) GetValueAtTime(f *clientmodel.Fingerprint, t clientmodel.Timestamp) Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.getValueAtTime(t)
}

func (s *memorySeriesStorage) GetBoundaryValues(f *clientmodel.Fingerprint, i Interval) Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.getBoundaryValues(i)
}

func (s *memorySeriesStorage) GetRangeValues(f *clientmodel.Fingerprint, i Interval) Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]

	if !ok {
		return nil
	}

	return series.getRangeValues(i)
}

func (s *memorySeriesStorage) Close() {
	s.Lock()
	defer s.Unlock()

	s.fingerprintToSeries = nil
	s.labelPairToFingerprints = nil
}

func (s *memorySeriesStorage) GetAllValuesForLabel(labelName clientmodel.LabelName) (values clientmodel.LabelValues, err error) {
	s.RLock()
	defer s.RUnlock()

	valueSet := map[clientmodel.LabelValue]bool{}
	for _, series := range s.fingerprintToSeries {
		if value, ok := series.metric()[labelName]; ok {
			if !valueSet[value] {
				values = append(values, value)
				valueSet[value] = true
			}
		}
	}

	return
}

// NewMemorySeriesStorage returns a memory series storage ready to use.
func NewMemorySeriesStorage(o MemorySeriesOptions) *memorySeriesStorage {
	return &memorySeriesStorage{
		fingerprintToSeries:     make(map[clientmodel.Fingerprint]stream),
		labelPairToFingerprints: make(map[LabelPair]utility.Set),
		wmCache:                 o.WatermarkCache,
	}
}
