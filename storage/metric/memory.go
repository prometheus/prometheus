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
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

// Assuming sample rate of 1 / 15Hz, this allows for one hour's worth of
// storage per metric without any major reallocations.
const initialSeriesArenaSize = 4 * 60

// Models a given sample entry stored in the in-memory arena.
type value interface {
	// Gets the given value.
	get() clientmodel.SampleValue
}

// Models a single sample value.  It presumes that there is either no subsequent
// value seen or that any subsequent values are of a different value.
type singletonValue clientmodel.SampleValue

func (v singletonValue) get() clientmodel.SampleValue {
	return clientmodel.SampleValue(v)
}

type stream struct {
	sync.RWMutex

	metric clientmodel.Metric
	values Values
}

func (s *stream) add(timestamp time.Time, value clientmodel.SampleValue) {
	s.Lock()
	defer s.Unlock()

	// BUG(all): https://github.com/prometheus/prometheus/pull/265/files#r4336435.

	s.values = append(s.values, &SamplePair{
		Timestamp: timestamp,
		Value:     value,
	})
}

func (s *stream) clone() Values {
	s.RLock()
	defer s.RUnlock()

	// BUG(all): Examine COW technique.

	clone := make(Values, len(s.values))
	copy(clone, s.values)

	return clone
}

func (s *stream) getValueAtTime(t time.Time) Values {
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

func (s *stream) getBoundaryValues(in Interval) Values {
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

func (s *stream) getRangeValues(in Interval) Values {
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

func (s *stream) empty() bool {
	return len(s.values) == 0
}

func newStream(metric clientmodel.Metric) *stream {
	return &stream{
		metric: metric,
		values: make(Values, 0, initialSeriesArenaSize),
	}
}

type memorySeriesStorage struct {
	sync.RWMutex

	wmCache                 *WatermarkCache
	fingerprintToSeries     map[clientmodel.Fingerprint]*stream
	labelPairToFingerprints map[LabelPair]clientmodel.Fingerprints
	labelNameToFingerprints map[clientmodel.LabelName]clientmodel.Fingerprints
}

type MemorySeriesOptions struct {
	// If provided, this WatermarkCache will be updated for any samples that are
	// appended to the memorySeriesStorage.
	WatermarkCache *WatermarkCache
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
	series.add(sample.Timestamp, sample.Value)

	if s.wmCache != nil {
		s.wmCache.Set(fingerprint, &watermarks{High: sample.Timestamp})
	}

	return nil
}

func (s *memorySeriesStorage) CreateEmptySeries(metric clientmodel.Metric) {
	s.Lock()
	defer s.Unlock()

	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromMetric(metric)
	s.getOrCreateSeries(metric, fingerprint)
}

func (s *memorySeriesStorage) getOrCreateSeries(metric clientmodel.Metric, fingerprint *clientmodel.Fingerprint) *stream {
	series, ok := s.fingerprintToSeries[*fingerprint]

	if !ok {
		series = newStream(metric)
		s.fingerprintToSeries[*fingerprint] = series

		for k, v := range metric {
			labelPair := LabelPair{
				Name:  k,
				Value: v,
			}
			labelPairValues := s.labelPairToFingerprints[labelPair]
			labelPairValues = append(labelPairValues, fingerprint)
			s.labelPairToFingerprints[labelPair] = labelPairValues

			labelNameValues := s.labelNameToFingerprints[k]
			labelNameValues = append(labelNameValues, fingerprint)
			s.labelNameToFingerprints[k] = labelNameValues
		}
	}
	return series
}

func (s *memorySeriesStorage) Flush(flushOlderThan time.Time, queue chan<- clientmodel.Samples) {
	emptySeries := []clientmodel.Fingerprint{}

	s.RLock()
	for fingerprint, stream := range s.fingerprintToSeries {
		finder := func(i int) bool {
			return stream.values[i].Timestamp.After(flushOlderThan)
		}

		stream.Lock()

		i := sort.Search(len(stream.values), finder)
		toArchive := stream.values[:i]
		toKeep := stream.values[i:]
		queued := make(clientmodel.Samples, 0, len(toArchive))

		for _, value := range toArchive {
			queued = append(queued, &clientmodel.Sample{
				Metric:    stream.metric,
				Timestamp: value.Timestamp,
				Value:     value.Value,
			})
		}

		// BUG(all): this can deadlock if the queue is full, as we only ever clear
		// the queue after calling this method:
		// https://github.com/prometheus/prometheus/issues/275
		queue <- queued

		stream.values = toKeep

		if len(toKeep) == 0 {
			emptySeries = append(emptySeries, fingerprint)
		}
		stream.Unlock()
	}
	s.RUnlock()

	s.Lock()
	for _, fingerprint := range emptySeries {
		if s.fingerprintToSeries[fingerprint].empty() {
			s.dropSeries(&fingerprint)
		}
	}
	s.Unlock()
}

// Drop all references to a series, including any samples.
func (s *memorySeriesStorage) dropSeries(fingerprint *clientmodel.Fingerprint) {
	series, ok := s.fingerprintToSeries[*fingerprint]
	if !ok {
		return
	}
	for k, v := range series.metric {
		labelPair := LabelPair{
			Name:  k,
			Value: v,
		}
		delete(s.labelPairToFingerprints, labelPair)
		delete(s.labelNameToFingerprints, k)
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
		series = newStream(clientmodel.Metric{})
		s.fingerprintToSeries[*fingerprint] = series
	}

	for _, sample := range samples {
		series.add(sample.Timestamp, sample.Value)
	}
}

func (s *memorySeriesStorage) GetFingerprintsForLabelSet(l clientmodel.LabelSet) (fingerprints clientmodel.Fingerprints, err error) {
	s.RLock()
	defer s.RUnlock()

	sets := []utility.Set{}
	for k, v := range l {
		values := s.labelPairToFingerprints[LabelPair{
			Name:  k,
			Value: v,
		}]
		set := utility.Set{}
		for _, fingerprint := range values {
			set.Add(*fingerprint)
		}
		sets = append(sets, set)
	}

	setCount := len(sets)
	if setCount == 0 {
		return fingerprints, nil
	}

	base := sets[0]
	for i := 1; i < setCount; i++ {
		base = base.Intersection(sets[i])
	}
	for _, e := range base.Elements() {
		fingerprint := e.(clientmodel.Fingerprint)
		fingerprints = append(fingerprints, &fingerprint)
	}

	return fingerprints, nil
}

func (s *memorySeriesStorage) GetFingerprintsForLabelName(l clientmodel.LabelName) (clientmodel.Fingerprints, error) {
	s.RLock()
	defer s.RUnlock()

	values, ok := s.labelNameToFingerprints[l]
	if !ok {
		return nil, nil
	}

	fingerprints := make(clientmodel.Fingerprints, len(values))
	copy(fingerprints, values)

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
	for label, value := range series.metric {
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

func (s *memorySeriesStorage) GetValueAtTime(f *clientmodel.Fingerprint, t time.Time) Values {
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

	s.fingerprintToSeries = map[clientmodel.Fingerprint]*stream{}
	s.labelPairToFingerprints = map[LabelPair]clientmodel.Fingerprints{}
	s.labelNameToFingerprints = map[clientmodel.LabelName]clientmodel.Fingerprints{}
}

func (s *memorySeriesStorage) GetAllValuesForLabel(labelName clientmodel.LabelName) (values clientmodel.LabelValues, err error) {
	s.RLock()
	defer s.RUnlock()

	valueSet := map[clientmodel.LabelValue]bool{}
	for _, series := range s.fingerprintToSeries {
		if value, ok := series.metric[labelName]; ok {
			if !valueSet[value] {
				values = append(values, value)
				valueSet[value] = true
			}
		}
	}

	return
}

func NewMemorySeriesStorage(o MemorySeriesOptions) *memorySeriesStorage {
	return &memorySeriesStorage{
		fingerprintToSeries:     make(map[clientmodel.Fingerprint]*stream),
		labelPairToFingerprints: make(map[LabelPair]clientmodel.Fingerprints),
		labelNameToFingerprints: make(map[clientmodel.LabelName]clientmodel.Fingerprints),
		wmCache:                 o.WatermarkCache,
	}
}
