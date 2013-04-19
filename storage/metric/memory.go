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
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/utility"
	"github.com/ryszard/goskiplist/skiplist"
	"time"
)

const (
	// Used as a separator in the format string for generating the internal label
	// value pair set fingerprints.
	reservedDelimiter = `"`
)

// Models a given sample entry stored in the in-memory arena.
type value interface {
	// Gets the given value.
	get() model.SampleValue
}

// Models a single sample value.  It presumes that there is either no subsequent
// value seen or that any subsequent values are of a different value.
type singletonValue model.SampleValue

func (v singletonValue) get() model.SampleValue {
	return model.SampleValue(v)
}

type skipListTime time.Time

func (t skipListTime) LessThan(o skiplist.Ordered) bool {
	return time.Time(o.(skipListTime)).Before(time.Time(t))
}

type stream struct {
	metric model.Metric
	values *skiplist.SkipList
}

func (s stream) add(timestamp time.Time, value model.SampleValue) {
	s.values.Set(skipListTime(timestamp), singletonValue(value))
}

func (s stream) forEach(decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) (scannedEntireCorpus bool, err error) {
	if s.values.Len() == 0 {
		return
	}
	iterator := s.values.SeekToLast()

	defer iterator.Close()

	for !(iterator.Key() == nil || iterator.Value() == nil) {
		decodedKey, decodeErr := decoder.DecodeKey(iterator.Key())
		if decodeErr != nil {
			panic(decodeErr)
		}
		decodedValue, decodeErr := decoder.DecodeValue(iterator.Value())
		if decodeErr != nil {
			panic(decodeErr)
		}

		switch filter.Filter(decodedKey, decodedValue) {
		case storage.STOP:
			return
		case storage.SKIP:
			continue
		case storage.ACCEPT:
			opErr := operator.Operate(decodedKey, decodedValue)
			if opErr != nil {
				if opErr.Continuable {
					continue
				}
				break
			}
		}
		if !iterator.Previous() {
			break
		}
	}
	scannedEntireCorpus = true
	return
}

func newStream(metric model.Metric) stream {
	return stream{
		values: skiplist.New(),
		metric: metric,
	}
}

type memorySeriesStorage struct {
	fingerprintToSeries     map[model.Fingerprint]stream
	labelPairToFingerprints map[string]model.Fingerprints
	labelNameToFingerprints map[model.LabelName]model.Fingerprints
}

func (s memorySeriesStorage) AppendSamples(samples model.Samples) (err error) {
	for _, sample := range samples {
		s.AppendSample(sample)
	}

	return
}

func (s memorySeriesStorage) AppendSample(sample model.Sample) (err error) {
	var (
		metric      = sample.Metric
		fingerprint = model.NewFingerprintFromMetric(metric)
		series, ok  = s.fingerprintToSeries[fingerprint]
	)

	if !ok {
		series = newStream(metric)
		s.fingerprintToSeries[fingerprint] = series

		for k, v := range metric {
			labelPair := fmt.Sprintf("%s%s%s", k, reservedDelimiter, v)

			labelPairValues := s.labelPairToFingerprints[labelPair]
			labelPairValues = append(labelPairValues, fingerprint)
			s.labelPairToFingerprints[labelPair] = labelPairValues

			labelNameValues := s.labelNameToFingerprints[k]
			labelNameValues = append(labelNameValues, fingerprint)
			s.labelNameToFingerprints[k] = labelNameValues
		}
	}

	series.add(sample.Timestamp, sample.Value)

	return
}

// Append raw sample, bypassing indexing. Only used to add data to views, which
// don't need to lookup by metric.
func (s memorySeriesStorage) appendSampleWithoutIndexing(f model.Fingerprint, timestamp time.Time, value model.SampleValue) {
	series, ok := s.fingerprintToSeries[f]

	if !ok {
		series = newStream(model.Metric{})
		s.fingerprintToSeries[f] = series
	}

	series.add(timestamp, value)
}

func (s memorySeriesStorage) GetFingerprintsForLabelSet(l model.LabelSet) (fingerprints model.Fingerprints, err error) {

	sets := []utility.Set{}

	for k, v := range l {
		signature := fmt.Sprintf("%s%s%s", k, reservedDelimiter, v)
		values := s.labelPairToFingerprints[signature]
		set := utility.Set{}
		for _, fingerprint := range values {
			set.Add(fingerprint)
		}
		sets = append(sets, set)
	}

	setCount := len(sets)
	if setCount == 0 {
		return
	}

	base := sets[0]
	for i := 1; i < setCount; i++ {
		base = base.Intersection(sets[i])
	}
	for _, e := range base.Elements() {
		fingerprint := e.(model.Fingerprint)
		fingerprints = append(fingerprints, fingerprint)
	}

	return
}

func (s memorySeriesStorage) GetFingerprintsForLabelName(l model.LabelName) (fingerprints model.Fingerprints, err error) {
	values := s.labelNameToFingerprints[l]

	fingerprints = append(fingerprints, values...)

	return
}

func (s memorySeriesStorage) GetMetricForFingerprint(f model.Fingerprint) (metric *model.Metric, err error) {
	series, ok := s.fingerprintToSeries[f]
	if !ok {
		return
	}

	metric = &series.metric

	return
}

func (s memorySeriesStorage) GetValueAtTime(f model.Fingerprint, t time.Time) (samples model.Values) {
	series, ok := s.fingerprintToSeries[f]
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

	foundTime := time.Time(iterator.Key().(skipListTime))
	samples = append(samples, model.SamplePair{
		Timestamp: foundTime,
		Value:     iterator.Value().(value).get(),
	})

	if foundTime.Before(t) && iterator.Previous() {
		samples = append(samples, model.SamplePair{
			Timestamp: time.Time(iterator.Key().(skipListTime)),
			Value:     iterator.Value().(value).get(),
		})
	}

	return
}

func (s memorySeriesStorage) GetBoundaryValues(f model.Fingerprint, i model.Interval) (first model.Values, second model.Values) {
	first = s.GetValueAtTime(f, i.OldestInclusive)
	second = s.GetValueAtTime(f, i.NewestInclusive)
	return
}

func (s memorySeriesStorage) GetRangeValues(f model.Fingerprint, i model.Interval) (samples model.Values) {
	series, ok := s.fingerprintToSeries[f]
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

	defer iterator.Close()

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

func (s memorySeriesStorage) Close() {
	s.fingerprintToSeries = map[model.Fingerprint]stream{}
	s.labelPairToFingerprints = map[string]model.Fingerprints{}
	s.labelNameToFingerprints = map[model.LabelName]model.Fingerprints{}
}

func (s memorySeriesStorage) GetAllValuesForLabel(labelName model.LabelName) (values model.LabelValues, err error) {
	valueSet := map[model.LabelValue]bool{}
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

func (s memorySeriesStorage) ForEachSample(builder IteratorsForFingerprintBuilder) (err error) {
	for _, stream := range s.fingerprintToSeries {
		decoder, filter, operator := builder.ForStream(stream)

		stream.forEach(decoder, filter, operator)
	}

	return
}

func NewMemorySeriesStorage() memorySeriesStorage {
	return memorySeriesStorage{
		fingerprintToSeries:     make(map[model.Fingerprint]stream),
		labelPairToFingerprints: make(map[string]model.Fingerprints),
		labelNameToFingerprints: make(map[model.LabelName]model.Fingerprints),
	}
}
