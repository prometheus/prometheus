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
	"code.google.com/p/goprotobuf/proto"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/utility"
	"time"
)

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

func (l *LevelDBMetricPersistence) hasIndexMetric(dto *dto.Metric) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: hasIndexMetric, result: success}, map[string]string{operation: hasIndexMetric, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	value, err = l.metricMembershipIndex.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) indexMetric(dto *dto.Metric) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: indexMetric, result: success}, map[string]string{operation: indexMetric, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	err = l.metricMembershipIndex.Put(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *dto.LabelPair) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	value, err = l.labelSetToFingerprints.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *dto.LabelName) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: hasLabelName, result: success}, map[string]string{operation: hasLabelName, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	value, err = l.labelNameToFingerprints.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) getFingerprintsForLabelSet(p *dto.LabelPair) (c *dto.FingerprintCollection, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}()

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
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getLabelNameFingerprints, result: success}, map[string]string{operation: getLabelNameFingerprints, result: failure})
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
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}()

	sets := []utility.Set{}

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

		set := utility.Set{}

		for _, m := range unmarshaled.Member {
			fp := model.Fingerprint(*m.Signature)
			set.Add(fp)
		}

		sets = append(sets, set)
	}

	numberOfSets := len(sets)
	if numberOfSets == 0 {
		return
	}

	base := sets[0]
	for i := 1; i < numberOfSets; i++ {
		base = base.Intersection(sets[i])
	}
	fps = []*model.Fingerprint{}
	for _, e := range base.Elements() {
		fingerprint := e.(model.Fingerprint)
		fps = append(fps, &fingerprint)
	}

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName *model.LabelName) (fps []*model.Fingerprint, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getFingerprintsForLabelName, result: success}, map[string]string{operation: getFingerprintsForLabelName, result: failure})
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
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getMetricForFingerprint, result: success}, map[string]string{operation: getMetricForFingerprint, result: failure})
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
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getBoundaryValues, result: success}, map[string]string{operation: getBoundaryValues, result: failure})
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

func (l *LevelDBMetricPersistence) GetValueAtTime(m *model.Metric, t *time.Time, s *metric.StalenessPolicy) (sample *model.Sample, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getValueAtTime, result: success}, map[string]string{operation: getValueAtTime, result: failure})
	}()

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
	if !iterator.Valid() {
		/*
		 * Two cases for this:
		 * 1.) Corruption in LevelDB.
		 * 2.) Key seek after AND outside known range.
		 *
		 * Once a LevelDB iterator goes invalid, it cannot be recovered; thusly,
		 * we need to create a new in order to check if the last value in the
		 * database is sufficient for our purposes.  This is, in all reality, a
		 * corner case but one that could bring down the system.
		 */
		iterator, closer, err = l.metricSamples.GetIterator()
		if err != nil {
			return
		}
		defer closer.Close()
		iterator.SeekToLast()
		if !iterator.Valid() {
			/*
			 * For whatever reason, the LevelDB cannot be recovered.
			 */
			return
		}
	}

	var (
		firstKey   *dto.SampleKey
		firstValue *dto.SampleValue
	)

	firstKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	peekAhead := false

	if !fingerprintsEqual(firstKey.Fingerprint, k.Fingerprint) {
		/*
		 * This allows us to grab values for metrics if our request time is after
		 * the last recorded time subject to the staleness policy due to the nuances
		 * of LevelDB storage:
		 *
		 * # Assumptions:
		 * - K0 < K1 in terms of sorting.
		 * - T0 < T1 in terms of sorting.
		 *
		 * # Data
		 *
		 * K0-T0
		 * K0-T1
		 * K0-T2
		 * K1-T0
		 * K1-T1
		 *
		 * # Scenario
		 * K0-T3, which does not exist, is requested.  LevelDB will thusly seek to
		 * K1-T1, when K0-T2 exists as a perfectly good candidate to check subject
		 * to the provided staleness policy and such.
		 */
		peekAhead = true
	}

	firstTime := indexable.DecodeTime(firstKey.Timestamp)
	if t.Before(firstTime) || peekAhead {
		iterator.Prev()
		if !iterator.Valid() {
			/*
			 * Two cases for this:
			 * 1.) Corruption in LevelDB.
			 * 2.) Key seek before AND outside known range.
			 *
			 * This is an explicit validation to ensure that if no previous values for
			 * the series are found, the query aborts.
			 */
			return
		}

		var (
			alternativeKey   *dto.SampleKey
			alternativeValue *dto.SampleValue
		)

		alternativeKey, err = extractSampleKey(iterator)
		if err != nil {
			return
		}

		if !fingerprintsEqual(alternativeKey.Fingerprint, k.Fingerprint) {
			return
		}

		/*
		 * At this point, we found a previous value in the same series in the
		 * database.  LevelDB originally seeked to the subsequent element given
		 * the key, but we need to consider this adjacency instead.
		 */
		alternativeTime := indexable.DecodeTime(alternativeKey.Timestamp)

		firstKey = alternativeKey
		firstValue = alternativeValue
		firstTime = alternativeTime
	}

	firstDelta := firstTime.Sub(*t)
	if firstDelta < 0 {
		firstDelta *= -1
	}
	if firstDelta > s.DeltaAllowance {
		return
	}

	firstValue, err = extractSampleValue(iterator)
	if err != nil {
		return
	}

	sample = model.SampleFromDTO(m, t, firstValue)

	if firstDelta == time.Duration(0) {
		return
	}

	iterator.Next()
	if !iterator.Valid() {
		/*
		 * Two cases for this:
		 * 1.) Corruption in LevelDB.
		 * 2.) Key seek after AND outside known range.
		 *
		 * This means that there are no more values left in the storage; and if this
		 * point is reached, we know that the one that has been found is within the
		 * allowed staleness limits.
		 */
		return
	}

	var secondKey *dto.SampleKey

	secondKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	if !fingerprintsEqual(secondKey.Fingerprint, k.Fingerprint) {
		return
	} else {
		/*
		 * At this point, current entry in the database has the same key as the
		 * previous.  For this reason, the validation logic will expect that the
		 * distance between the two points shall not exceed the staleness policy
		 * allowed limit to reduce interpolation errors.
		 *
		 * For this reason, the sample is reset in case of other subsequent
		 * validation behaviors.
		 */
		sample = nil
	}

	secondTime := indexable.DecodeTime(secondKey.Timestamp)

	totalDelta := secondTime.Sub(firstTime)
	if totalDelta > s.DeltaAllowance {
		return
	}

	var secondValue *dto.SampleValue

	secondValue, err = extractSampleValue(iterator)
	if err != nil {
		return
	}

	interpolated := interpolate(firstTime, secondTime, *firstValue.Value, *secondValue.Value, *t)

	sampleValue := &dto.SampleValue{
		Value: &interpolated,
	}

	sample = model.SampleFromDTO(m, t, sampleValue)

	return
}

func (l *LevelDBMetricPersistence) GetRangeValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (v *model.SampleSet, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: getRangeValues, result: success}, map[string]string{operation: getRangeValues, result: failure})
	}()

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

func (l *LevelDBMetricPersistence) GetAllMetricNames() (metricNames []string, err error) {
	metricNameMap := map[string]bool{}

	err = l.labelSetToFingerprints.ForAll(func(pair *raw.Pair) {
		unmarshaled := &dto.LabelPair{}
		err = proto.Unmarshal(pair.Left, unmarshaled)
		if err != nil {
			return
		}

		if *unmarshaled.Name == "name" {
			metricNameMap[*unmarshaled.Value] = true
		}
	})
	if err != nil {
		return
	}

	for labelName := range metricNameMap {
		metricNames = append(metricNames, labelName)
	}
	return
}
