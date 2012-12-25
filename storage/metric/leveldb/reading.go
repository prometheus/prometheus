// Copyright 2012 Prometheus Team
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

package leveldb

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	registry "github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/golang_instrumentation/metrics"
	"github.com/matttproud/prometheus/coding"
	"github.com/matttproud/prometheus/coding/indexable"
	"github.com/matttproud/prometheus/model"
	dto "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/storage/metric"
	"time"
)

var (
	getLabelNameFingerprintsSuccessCount    = &metrics.CounterMetric{}
	getLabelNameFingerprintsFailureCount    = &metrics.CounterMetric{}
	getFingerprintsForLabelSetSuccessCount  = &metrics.CounterMetric{}
	getFingerprintsForLabelSetFailureCount  = &metrics.CounterMetric{}
	getFingerprintsForLabelNameSuccessCount = &metrics.CounterMetric{}
	getFingerprintsForLabelNameFailureCount = &metrics.CounterMetric{}
	getMetricForFingerprintSuccessCount     = &metrics.CounterMetric{}
	getMetricForFingerprintFailureCount     = &metrics.CounterMetric{}
	getBoundaryValuesSuccessCount           = &metrics.CounterMetric{}
	getBoundaryValuesFailureCount           = &metrics.CounterMetric{}
)

func init() {
	registry.Register("get_label_name_fingerprints_success_count_total", getLabelNameFingerprintsSuccessCount)
	registry.Register("get_label_name_fingerprints_failure_count_total", getLabelNameFingerprintsFailureCount)

	registry.Register("get_fingerprints_for_label_set_success_count_total", getFingerprintsForLabelSetSuccessCount)
	registry.Register("get_fingerprints_for_label_set_failure_count_total", getFingerprintsForLabelSetFailureCount)
	registry.Register("get_fingerprints_for_label_name_success_count_total", getFingerprintsForLabelNameSuccessCount)
	registry.Register("get_fingerprints_for_label_name_failure_count_total", getFingerprintsForLabelNameFailureCount)
	registry.Register("get_metric_for_fingerprint_success_count_total", getMetricForFingerprintSuccessCount)
	registry.Register("get_metric_for_fingerprint_failure_count_total", getMetricForFingerprintFailureCount)
	registry.Register("get_boundary_values_success_count_total", getBoundaryValuesSuccessCount)
	registry.Register("get_boundary_values_failure_count_total", getBoundaryValuesFailureCount)
}

func extractSampleKey(i iterator) (k *dto.SampleKey, err error) {
	k = &dto.SampleKey{}
	err = proto.Unmarshal(i.Key(), k)

	return
}

func extractSampleValue(i iterator) (v *dto.SampleValue, err error) {
	v = &dto.SampleValue{}
	err = proto.Unmarshal(i.Value(), v)

	return
}

func fingerprintsEqual(l *dto.Fingerprint, r *dto.Fingerprint) bool {
	if l == r {
		return true
	}

	if l == nil && r == nil {
		return true
	}

	if r.Signature == l.Signature {
		return true
	}

	if *r.Signature == *l.Signature {
		return true
	}

	return false
}

type sampleKeyPredicate func(k *dto.SampleKey) bool

func keyIsOlderThan(t time.Time) sampleKeyPredicate {
	unix := t.Unix()

	return func(k *dto.SampleKey) bool {
		return indexable.DecodeTime(k.Timestamp).Unix() > unix
	}
}

func keyIsAtMostOld(t time.Time) sampleKeyPredicate {
	unix := t.Unix()

	return func(k *dto.SampleKey) bool {
		return indexable.DecodeTime(k.Timestamp).Unix() <= unix
	}
}

func (l *LevelDBMetricPersistence) hasIndexMetric(dto *dto.Metric) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.metricMembershipIndex.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) indexMetric(dto *dto.Metric) error {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.metricMembershipIndex.Put(dtoKey)
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *dto.LabelPair) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.labelSetToFingerprints.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *dto.LabelName) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.labelNameToFingerprints.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) getFingerprintsForLabelSet(p *dto.LabelPair) (c *dto.FingerprintCollection, err error) {
	dtoKey := coding.NewProtocolBufferEncoder(p)
	get, err := l.labelSetToFingerprints.Get(dtoKey)
	if err != nil {
		return
	}

	c = &dto.FingerprintCollection{}
	err = proto.Unmarshal(get, c)

	return
}

// XXX: Delete me and replace with GetFingerprintsForLabelName.
func (l *LevelDBMetricPersistence) GetLabelNameFingerprints(n *dto.LabelName) (c *dto.FingerprintCollection, err error) {
	defer func() {
		m := getLabelNameFingerprintsSuccessCount
		if err != nil {
			m = getLabelNameFingerprintsFailureCount
		}

		m.Increment()
	}()

	dtoKey := coding.NewProtocolBufferEncoder(n)
	get, err := l.labelNameToFingerprints.Get(dtoKey)
	if err != nil {
		return
	}

	c = &dto.FingerprintCollection{}
	err = proto.Unmarshal(get, c)

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet *model.LabelSet) (fps []*model.Fingerprint, err error) {
	defer func() {
		m := getFingerprintsForLabelSetSuccessCount
		if err != nil {
			m = getFingerprintsForLabelSetFailureCount
		}

		m.Increment()
	}()

	fps = make([]*model.Fingerprint, 0, 0)

	for _, labelSetDTO := range model.LabelSetToDTOs(labelSet) {
		f, err := l.labelSetToFingerprints.Get(coding.NewProtocolBufferEncoder(labelSetDTO))
		if err != nil {
			return fps, err
		}

		unmarshaled := &dto.FingerprintCollection{}
		err = proto.Unmarshal(f, unmarshaled)
		if err != nil {
			return fps, err
		}

		for _, m := range unmarshaled.Member {
			fp := model.Fingerprint(*m.Signature)
			fps = append(fps, &fp)
		}
	}

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName *model.LabelName) (fps []*model.Fingerprint, err error) {
	defer func() {
		m := getFingerprintsForLabelNameSuccessCount
		if err != nil {
			m = getFingerprintsForLabelNameFailureCount
		}

		m.Increment()
	}()

	fps = make([]*model.Fingerprint, 0, 0)

	raw, err := l.labelNameToFingerprints.Get(coding.NewProtocolBufferEncoder(model.LabelNameToDTO(labelName)))
	if err != nil {
		return
	}

	unmarshaled := &dto.FingerprintCollection{}

	err = proto.Unmarshal(raw, unmarshaled)
	if err != nil {
		return
	}

	for _, m := range unmarshaled.Member {
		fp := model.Fingerprint(*m.Signature)
		fps = append(fps, &fp)
	}

	return
}

func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f *model.Fingerprint) (m *model.Metric, err error) {
	defer func() {
		m := getMetricForFingerprintSuccessCount
		if err != nil {
			m = getMetricForFingerprintFailureCount
		}

		m.Increment()
	}()

	raw, err := l.fingerprintToMetrics.Get(coding.NewProtocolBufferEncoder(model.FingerprintToDTO(f)))
	if err != nil {
		return
	}

	unmarshaled := &dto.Metric{}
	err = proto.Unmarshal(raw, unmarshaled)
	if err != nil {
		return
	}

	m = &model.Metric{}
	for _, v := range unmarshaled.LabelPair {
		(*m)[model.LabelName(*v.Name)] = model.LabelValue(*v.Value)
	}

	return
}

func (l *LevelDBMetricPersistence) GetBoundaryValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (open *model.Sample, end *model.Sample, err error) {
	defer func() {
		m := getBoundaryValuesSuccessCount
		if err != nil {
			m = getBoundaryValuesFailureCount
		}

		m.Increment()
	}()

	// XXX: Maybe we will want to emit incomplete sets?
	open, err = l.GetValueAtTime(m, &i.OldestInclusive, s)
	if err != nil {
		return
	} else if open == nil {
		return
	}

	end, err = l.GetValueAtTime(m, &i.NewestInclusive, s)
	if err != nil {
		return
	} else if end == nil {
		open = nil
	}

	return
}

func interpolate(x1, x2 time.Time, y1, y2 float32, e time.Time) float32 {
	yDelta := y2 - y1
	xDelta := x2.Sub(x1)

	dDt := yDelta / float32(xDelta)
	offset := float32(e.Sub(x1))

	return y1 + (offset * dDt)
}

type iterator interface {
	Close()
	Key() []byte
	Next()
	Prev()
	Seek([]byte)
	SeekToFirst()
	SeekToLast()
	Valid() bool
	Value() []byte
}

func isKeyInsideRecordedInterval(k *dto.SampleKey, i iterator) (b bool, err error) {
	byteKey, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	i.Seek(byteKey)
	if !i.Valid() {
		return
	}

	var (
		retrievedKey *dto.SampleKey
	)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		return
	}

	if !fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint) {
		return
	}

	if bytes.Equal(retrievedKey.Timestamp, k.Timestamp) {
		return true, nil
	}

	i.Prev()
	if !i.Valid() {
		return
	}

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		return
	}

	b = fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint)

	return
}

func doesKeyHavePrecursor(k *dto.SampleKey, i iterator) (b bool, err error) {
	byteKey, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	i.Seek(byteKey)

	if !i.Valid() {
		i.SeekToFirst()
	}

	var (
		retrievedKey *dto.SampleKey
	)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		return
	}

	if !fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint) {
		return
	}

	keyTime := indexable.DecodeTime(k.Timestamp)
	retrievedTime := indexable.DecodeTime(retrievedKey.Timestamp)

	return retrievedTime.Before(keyTime), nil
}

func doesKeyHaveSuccessor(k *dto.SampleKey, i iterator) (b bool, err error) {
	byteKey, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	i.Seek(byteKey)

	if !i.Valid() {
		i.SeekToLast()
	}

	var (
		retrievedKey *dto.SampleKey
	)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		return
	}

	if !fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint) {
		return
	}

	keyTime := indexable.DecodeTime(k.Timestamp)
	retrievedTime := indexable.DecodeTime(retrievedKey.Timestamp)

	return retrievedTime.After(keyTime), nil
}

func (l *LevelDBMetricPersistence) GetValueAtTime(m *model.Metric, t *time.Time, s *metric.StalenessPolicy) (sample *model.Sample, err error) {
	d := model.MetricToDTO(m)

	f, err := model.MessageToFingerprintDTO(d)
	if err != nil {
		return
	}

	// Candidate for Refactoring
	k := &dto.SampleKey{
		Fingerprint: f,
		Timestamp:   indexable.EncodeTime(*t),
	}

	e, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	iterator, closer, err := l.metricSamples.GetIterator()
	if err != nil {
		return
	}
	defer closer.Close()

	iterator.Seek(e)

	within, err := isKeyInsideRecordedInterval(k, iterator)
	if err != nil || !within {
		return
	}

	var (
		firstKey   *dto.SampleKey
		firstValue *dto.SampleValue
	)

	firstKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	if fingerprintsEqual(firstKey.Fingerprint, k.Fingerprint) {
		firstValue, err = extractSampleValue(iterator)

		if err != nil {
			return nil, err
		}

		foundTimestamp := indexable.DecodeTime(firstKey.Timestamp)
		targetTimestamp := indexable.DecodeTime(k.Timestamp)

		if foundTimestamp.Equal(targetTimestamp) {
			return model.SampleFromDTO(m, t, firstValue), nil
		}
	} else {
		return
	}

	var (
		secondKey   *dto.SampleKey
		secondValue *dto.SampleValue
	)

	iterator.Next()
	if !iterator.Valid() {
		return
	}

	secondKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	if fingerprintsEqual(secondKey.Fingerprint, k.Fingerprint) {
		secondValue, err = extractSampleValue(iterator)
		if err != nil {
			return
		}
	} else {
		return
	}

	firstTime := indexable.DecodeTime(firstKey.Timestamp)
	secondTime := indexable.DecodeTime(secondKey.Timestamp)
	currentDelta := secondTime.Sub(firstTime)

	if currentDelta <= s.DeltaAllowance {
		interpolated := interpolate(firstTime, secondTime, *firstValue.Value, *secondValue.Value, *t)
		emission := &dto.SampleValue{
			Value: &interpolated,
		}

		return model.SampleFromDTO(m, t, emission), nil
	}

	return
}

func (l *LevelDBMetricPersistence) GetRangeValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (v *model.SampleSet, err error) {
	d := model.MetricToDTO(m)

	f, err := model.MessageToFingerprintDTO(d)
	if err != nil {
		return
	}

	k := &dto.SampleKey{
		Fingerprint: f,
		Timestamp:   indexable.EncodeTime(i.OldestInclusive),
	}

	e, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	iterator, closer, err := l.metricSamples.GetIterator()
	if err != nil {
		return
	}
	defer closer.Close()

	iterator.Seek(e)

	predicate := keyIsOlderThan(i.NewestInclusive)

	for ; iterator.Valid(); iterator.Next() {
		retrievedKey := &dto.SampleKey{}

		retrievedKey, err = extractSampleKey(iterator)
		if err != nil {
			return
		}

		if predicate(retrievedKey) {
			break
		}

		if !fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint) {
			break
		}

		retrievedValue, err := extractSampleValue(iterator)
		if err != nil {
			return nil, err
		}

		if v == nil {
			v = &model.SampleSet{}
		}

		v.Values = append(v.Values, model.SamplePair{
			Value:     model.SampleValue(*retrievedValue.Value),
			Timestamp: indexable.DecodeTime(retrievedKey.Timestamp),
		})
	}

	return
}
