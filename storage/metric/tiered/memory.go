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
	"sort"
	"sync"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

// An initialSeriesArenaSize of 4*60 allows for one hour's worth of storage per
// metric without any major reallocations - assuming a sample rate of 1 / 15Hz.
const initialSeriesArenaSize = 4 * 60

type stream interface {
	add(metric.Values)

	clone() metric.Values
	getOlderThan(age clientmodel.Timestamp) metric.Values
	evictOlderThan(age clientmodel.Timestamp)

	size() int
	clear()

	metric() clientmodel.Metric

	getValueAtTime(t clientmodel.Timestamp) metric.Values
	getBoundaryValues(in metric.Interval) metric.Values
	getRangeValues(in metric.Interval) metric.Values
}

type arrayStream struct {
	sync.RWMutex

	m      clientmodel.Metric
	values metric.Values
}

func (s *arrayStream) metric() clientmodel.Metric {
	return s.m
}

// add implements the stream interface. This implementation requires both
// s.values and the passed in v to be sorted already. Values in v that have a
// timestamp older than the most recent value in s.values are skipped.
func (s *arrayStream) add(v metric.Values) {
	s.Lock()
	defer s.Unlock()
	// Skip over values that are older than the most recent value in s.
	if len(s.values) > 0 {
		i := 0
		mostRecentTimestamp := s.values[len(s.values)-1].Timestamp
		for ; i < len(v) && mostRecentTimestamp > v[i].Timestamp; i++ {
		}
		if i > 0 {
			glog.Warningf(
				"Skipped out-of-order values while adding to %#v: %#v",
				s.m, v[:i],
			)
			v = v[i:]
		}
	}
	s.values = append(s.values, v...)
}

func (s *arrayStream) clone() metric.Values {
	s.RLock()
	defer s.RUnlock()

	clone := make(metric.Values, len(s.values))
	copy(clone, s.values)

	return clone
}

func (s *arrayStream) getOlderThan(t clientmodel.Timestamp) metric.Values {
	s.RLock()
	defer s.RUnlock()

	finder := func(i int) bool {
		return s.values[i].Timestamp.After(t)
	}

	i := sort.Search(len(s.values), finder)
	return s.values[:i]
}

func (s *arrayStream) evictOlderThan(t clientmodel.Timestamp) {
	s.Lock()
	defer s.Unlock()

	finder := func(i int) bool {
		return s.values[i].Timestamp.After(t)
	}

	i := sort.Search(len(s.values), finder)
	s.values = s.values[i:]
}

func (s *arrayStream) getValueAtTime(t clientmodel.Timestamp) metric.Values {
	s.RLock()
	defer s.RUnlock()

	// BUG(all): May be avenues for simplification.
	l := len(s.values)
	switch l {
	case 0:
		return metric.Values{}
	case 1:
		return metric.Values{s.values[0]}
	default:
		index := sort.Search(l, func(i int) bool {
			return !s.values[i].Timestamp.Before(t)
		})

		if index == 0 {
			return metric.Values{s.values[0]}
		}
		if index == l {
			return metric.Values{s.values[l-1]}
		}

		if s.values[index].Timestamp.Equal(t) {
			return metric.Values{s.values[index]}
		}
		return metric.Values{s.values[index-1], s.values[index]}
	}
}

func (s *arrayStream) getBoundaryValues(in metric.Interval) metric.Values {
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
		return metric.Values{}
	case 1:
		return metric.Values{resultRange[0]}
	default:
		return metric.Values{resultRange[0], resultRange[len(resultRange)-1]}
	}
}

func (s *arrayStream) getRangeValues(in metric.Interval) metric.Values {
	s.RLock()
	defer s.RUnlock()

	oldest := sort.Search(len(s.values), func(i int) bool {
		return !s.values[i].Timestamp.Before(in.OldestInclusive)
	})

	newest := sort.Search(len(s.values), func(i int) bool {
		return s.values[i].Timestamp.After(in.NewestInclusive)
	})

	result := make(metric.Values, newest-oldest)
	copy(result, s.values[oldest:newest])

	return result
}

func (s *arrayStream) size() int {
	return len(s.values)
}

func (s *arrayStream) clear() {
	s.values = metric.Values{}
}

func newArrayStream(m clientmodel.Metric) *arrayStream {
	return &arrayStream{
		m:      m,
		values: make(metric.Values, 0, initialSeriesArenaSize),
	}
}

type memorySeriesStorage struct {
	sync.RWMutex

	wmCache                 *watermarkCache
	fingerprintToSeries     map[clientmodel.Fingerprint]stream
	labelPairToFingerprints map[metric.LabelPair]utility.Set
	labelNameToLabelValues  map[clientmodel.LabelName]utility.Set
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
	series.add(metric.Values{
		metric.SamplePair{
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

func (s *memorySeriesStorage) getOrCreateSeries(m clientmodel.Metric, fp *clientmodel.Fingerprint) stream {
	series, ok := s.fingerprintToSeries[*fp]

	if !ok {
		series = newArrayStream(m)
		s.fingerprintToSeries[*fp] = series

		for k, v := range m {
			labelPair := metric.LabelPair{
				Name:  k,
				Value: v,
			}

			fps, ok := s.labelPairToFingerprints[labelPair]
			if !ok {
				fps = utility.Set{}
				s.labelPairToFingerprints[labelPair] = fps
			}
			fps.Add(*fp)

			values, ok := s.labelNameToLabelValues[k]
			if !ok {
				values = utility.Set{}
				s.labelNameToLabelValues[k] = values
			}
			values.Add(v)
		}
	}
	return series
}

func (s *memorySeriesStorage) Flush(flushOlderThan clientmodel.Timestamp, queue chan<- clientmodel.Samples) {
	s.RLock()
	for _, stream := range s.fingerprintToSeries {
		toArchive := stream.getOlderThan(flushOlderThan)
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
	}
	s.RUnlock()
}

func (s *memorySeriesStorage) Evict(flushOlderThan clientmodel.Timestamp) {
	emptySeries := []clientmodel.Fingerprint{}

	s.RLock()
	for fingerprint, stream := range s.fingerprintToSeries {
		stream.evictOlderThan(flushOlderThan)
		if stream.size() == 0 {
			emptySeries = append(emptySeries, fingerprint)
		}
	}
	s.RUnlock()

	s.Lock()
	for _, fingerprint := range emptySeries {
		if series, ok := s.fingerprintToSeries[fingerprint]; ok && series.size() == 0 {
			s.dropSeries(&fingerprint)
		}
	}
	s.Unlock()
}

// Drop a label value from the label names to label values index.
func (s *memorySeriesStorage) dropLabelValue(l clientmodel.LabelName, v clientmodel.LabelValue) {
	if set, ok := s.labelNameToLabelValues[l]; ok {
		set.Remove(v)
		if len(set) == 0 {
			delete(s.labelNameToLabelValues, l)
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
		labelPair := metric.LabelPair{
			Name:  k,
			Value: v,
		}
		if set, ok := s.labelPairToFingerprints[labelPair]; ok {
			set.Remove(*fingerprint)
			if len(set) == 0 {
				delete(s.labelPairToFingerprints, labelPair)
				s.dropLabelValue(k, v)
			}
		}
	}
	delete(s.fingerprintToSeries, *fingerprint)
}

// Append raw samples, bypassing indexing. Only used to add data to views,
// which don't need to lookup by metric.
func (s *memorySeriesStorage) appendSamplesWithoutIndexing(fingerprint *clientmodel.Fingerprint, samples metric.Values) {
	s.Lock()
	defer s.Unlock()

	series, ok := s.fingerprintToSeries[*fingerprint]

	if !ok {
		series = newArrayStream(clientmodel.Metric{})
		s.fingerprintToSeries[*fingerprint] = series
	}

	series.add(samples)
}

func (s *memorySeriesStorage) GetFingerprintsForLabelMatchers(labelMatchers metric.LabelMatchers) (clientmodel.Fingerprints, error) {
	s.RLock()
	defer s.RUnlock()

	sets := []utility.Set{}
	for _, matcher := range labelMatchers {
		switch matcher.Type {
		case metric.Equal:
			set, ok := s.labelPairToFingerprints[metric.LabelPair{
				Name:  matcher.Name,
				Value: matcher.Value,
			}]
			if !ok {
				return nil, nil
			}
			sets = append(sets, set)
		default:
			values, err := s.getLabelValuesForLabelName(matcher.Name)
			if err != nil {
				return nil, err
			}

			matches := matcher.Filter(values)
			if len(matches) == 0 {
				return nil, nil
			}
			set := utility.Set{}
			for _, v := range matches {
				subset, ok := s.labelPairToFingerprints[metric.LabelPair{
					Name:  matcher.Name,
					Value: v,
				}]
				if !ok {
					return nil, nil
				}
				for fp := range subset {
					set.Add(fp)
				}
			}
			sets = append(sets, set)
		}
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

func (s *memorySeriesStorage) GetLabelValuesForLabelName(labelName clientmodel.LabelName) (clientmodel.LabelValues, error) {
	s.RLock()
	defer s.RUnlock()

	return s.getLabelValuesForLabelName(labelName)
}

func (s *memorySeriesStorage) getLabelValuesForLabelName(labelName clientmodel.LabelName) (clientmodel.LabelValues, error) {
	set, ok := s.labelNameToLabelValues[labelName]
	if !ok {
		return nil, nil
	}

	values := make(clientmodel.LabelValues, 0, len(set))
	for e := range set {
		val := e.(clientmodel.LabelValue)
		values = append(values, val)
	}
	return values, nil
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

func (s *memorySeriesStorage) CloneSamples(f *clientmodel.Fingerprint) metric.Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.clone()
}

func (s *memorySeriesStorage) GetValueAtTime(f *clientmodel.Fingerprint, t clientmodel.Timestamp) metric.Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.getValueAtTime(t)
}

func (s *memorySeriesStorage) GetBoundaryValues(f *clientmodel.Fingerprint, i metric.Interval) metric.Values {
	s.RLock()
	defer s.RUnlock()

	series, ok := s.fingerprintToSeries[*f]
	if !ok {
		return nil
	}

	return series.getBoundaryValues(i)
}

func (s *memorySeriesStorage) GetRangeValues(f *clientmodel.Fingerprint, i metric.Interval) metric.Values {
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
	s.labelNameToLabelValues = nil
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
		labelPairToFingerprints: make(map[metric.LabelPair]utility.Set),
		labelNameToLabelValues:  make(map[clientmodel.LabelName]utility.Set),
		wmCache:                 o.WatermarkCache,
	}
}
