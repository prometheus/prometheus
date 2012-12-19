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
	"errors"
	"github.com/matttproud/prometheus/coding"
	"github.com/matttproud/prometheus/coding/indexable"
	"github.com/matttproud/prometheus/model"
	dto "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/storage/metric"
	"log"
	"time"
)

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

func (l *LevelDBMetricPersistence) getFingerprintsForLabelSet(p *dto.LabelPair) (*dto.FingerprintCollection, error) {
	dtoKey := coding.NewProtocolBufferEncoder(p)
	if get, getError := l.labelSetToFingerprints.Get(dtoKey); getError == nil {
		value := &dto.FingerprintCollection{}
		if unmarshalError := proto.Unmarshal(get, value); unmarshalError == nil {
			return value, nil
		} else {
			return nil, unmarshalError
		}
	} else {
		return nil, getError
	}

	panic("unreachable")
}

func (l *LevelDBMetricPersistence) GetLabelNameFingerprints(n *dto.LabelName) (*dto.FingerprintCollection, error) {
	dtoKey := coding.NewProtocolBufferEncoder(n)
	if get, getError := l.labelNameToFingerprints.Get(dtoKey); getError == nil {
		value := &dto.FingerprintCollection{}
		if unmarshalError := proto.Unmarshal(get, value); unmarshalError == nil {
			return value, nil
		} else {
			return nil, unmarshalError
		}
	} else {
		return nil, getError
	}

	return nil, errors.New("Unknown error while getting label name fingerprints.")
}

func (l *LevelDBMetricPersistence) GetSamplesForMetric(metric model.Metric, interval model.Interval) ([]model.Samples, error) {
	metricDTO := model.MetricToDTO(&metric)

	if fingerprintDTO, fingerprintDTOErr := model.MessageToFingerprintDTO(metricDTO); fingerprintDTOErr == nil {
		if iterator, closer, iteratorErr := l.metricSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &dto.SampleKey{
				Fingerprint: fingerprintDTO,
				Timestamp:   indexable.EncodeTime(interval.OldestInclusive),
			}

			emission := make([]model.Samples, 0)

			if encode, encodeErr := coding.NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				for iterator = iterator; iterator.Valid(); iterator.Next() {
					key := &dto.SampleKey{}
					value := &dto.SampleValue{}
					if keyUnmarshalErr := proto.Unmarshal(iterator.Key(), key); keyUnmarshalErr == nil {
						if valueUnmarshalErr := proto.Unmarshal(iterator.Value(), value); valueUnmarshalErr == nil {
							if *fingerprintDTO.Signature == *key.Fingerprint.Signature {
								// Wart
								if indexable.DecodeTime(key.Timestamp).Unix() <= interval.NewestInclusive.Unix() {
									emission = append(emission, model.Samples{
										Value:     model.SampleValue(*value.Value),
										Timestamp: indexable.DecodeTime(key.Timestamp),
									})
								} else {
									break
								}
							} else {
								break
							}
						} else {
							return nil, valueUnmarshalErr
						}
					} else {
						return nil, keyUnmarshalErr
					}
				}

				return emission, nil

			} else {
				log.Printf("Could not encode the start key: %q\n", encodeErr)
				return nil, encodeErr
			}
		} else {
			log.Printf("Could not acquire iterator: %q\n", iteratorErr)
			return nil, iteratorErr
		}
	} else {
		log.Printf("Could not create fingerprint for the metric: %q\n", fingerprintDTOErr)
		return nil, fingerprintDTOErr
	}

	panic("unreachable")
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet *model.LabelSet) ([]*model.Fingerprint, error) {
	emission := make([]*model.Fingerprint, 0, 0)

	for _, labelSetDTO := range model.LabelSetToDTOs(labelSet) {
		if f, err := l.labelSetToFingerprints.Get(coding.NewProtocolBufferEncoder(labelSetDTO)); err == nil {
			unmarshaled := &dto.FingerprintCollection{}
			if unmarshalErr := proto.Unmarshal(f, unmarshaled); unmarshalErr == nil {
				for _, m := range unmarshaled.Member {
					fp := model.Fingerprint(*m.Signature)
					emission = append(emission, &fp)
				}
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return emission, nil
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName *model.LabelName) ([]*model.Fingerprint, error) {
	emission := make([]*model.Fingerprint, 0, 0)

	if raw, err := l.labelNameToFingerprints.Get(coding.NewProtocolBufferEncoder(model.LabelNameToDTO(labelName))); err == nil {

		unmarshaled := &dto.FingerprintCollection{}

		if err = proto.Unmarshal(raw, unmarshaled); err == nil {
			for _, m := range unmarshaled.Member {
				fp := model.Fingerprint(*m.Signature)
				emission = append(emission, &fp)
			}
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}

	return emission, nil
}

func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f *model.Fingerprint) (*model.Metric, error) {
	if raw, err := l.fingerprintToMetrics.Get(coding.NewProtocolBufferEncoder(model.FingerprintToDTO(f))); err == nil {
		unmarshaled := &dto.Metric{}
		if unmarshalErr := proto.Unmarshal(raw, unmarshaled); unmarshalErr == nil {
			m := model.Metric{}
			for _, v := range unmarshaled.LabelPair {
				m[model.LabelName(*v.Name)] = model.LabelValue(*v.Value)
			}
			return &m, nil
		} else {
			return nil, unmarshalErr
		}
	} else {
		return nil, err
	}

	panic("unreachable")
}

func (l *LevelDBMetricPersistence) GetBoundaryValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (*model.Sample, *model.Sample, error) {
	panic("not implemented")
}

func (l *LevelDBMetricPersistence) GetFirstValue(m *model.Metric) (*model.Sample, error) {
	panic("not implemented")
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
		retrievedKey *dto.SampleKey = &dto.SampleKey{}
	)

	err = proto.Unmarshal(i.Key(), retrievedKey)
	if err != nil {
		return
	}

	if *retrievedKey.Fingerprint.Signature != *k.Fingerprint.Signature {
		return
	}

	if bytes.Equal(retrievedKey.Timestamp, k.Timestamp) {
		return true, nil
	}

	i.Prev()
	if !i.Valid() {
		return
	}

	err = proto.Unmarshal(i.Key(), retrievedKey)
	if err != nil {
		return
	}

	b = *retrievedKey.Fingerprint.Signature == *k.Fingerprint.Signature

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
		retrievedKey *dto.SampleKey = &dto.SampleKey{}
	)

	err = proto.Unmarshal(i.Key(), retrievedKey)
	if err != nil {
		return
	}

	signaturesEqual := *retrievedKey.Fingerprint.Signature == *k.Fingerprint.Signature
	if !signaturesEqual {
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
		retrievedKey *dto.SampleKey = &dto.SampleKey{}
	)

	err = proto.Unmarshal(i.Key(), retrievedKey)
	if err != nil {
		return
	}

	signaturesEqual := *retrievedKey.Fingerprint.Signature == *k.Fingerprint.Signature
	if !signaturesEqual {
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

	var (
		firstKey   *dto.SampleKey   = &dto.SampleKey{}
		firstValue *dto.SampleValue = nil
	)

	within, err := isKeyInsideRecordedInterval(k, iterator)
	if err != nil || !within {
		return
	}

	for iterator = iterator; iterator.Valid(); iterator.Prev() {
		err := proto.Unmarshal(iterator.Key(), firstKey)
		if err != nil {
			return nil, err
		}

		if *firstKey.Fingerprint.Signature == *k.Fingerprint.Signature {
			firstValue = &dto.SampleValue{}
			err := proto.Unmarshal(iterator.Value(), firstValue)
			if err != nil {
				return nil, err
			}

			if indexable.DecodeTime(firstKey.Timestamp).Equal(indexable.DecodeTime(k.Timestamp)) {
				return model.SampleFromDTO(m, t, firstValue), nil
			}
			break
		}
	}

	var (
		secondKey   *dto.SampleKey   = &dto.SampleKey{}
		secondValue *dto.SampleValue = nil
	)

	iterator.Next()
	if !iterator.Valid() {
		return
	}

	err = proto.Unmarshal(iterator.Key(), secondKey)
	if err != nil {

		return
	}

	if *secondKey.Fingerprint.Signature == *k.Fingerprint.Signature {
		secondValue = &dto.SampleValue{}
		err = proto.Unmarshal(iterator.Value(), secondValue)
		if err != nil {
			return
		}
	}

	firstTime := indexable.DecodeTime(firstKey.Timestamp)
	secondTime := indexable.DecodeTime(secondKey.Timestamp)
	interpolated := interpolate(firstTime, secondTime, *firstValue.Value, *secondValue.Value, *t)
	emission := &dto.SampleValue{
		Value: &interpolated,
	}

	return model.SampleFromDTO(m, t, emission), nil
}

func (l *LevelDBMetricPersistence) GetRangeValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (*model.SampleSet, error) {
	panic("not implemented")
}
