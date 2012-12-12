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

func (l *LevelDBMetricPersistence) GetValueAtTime(m *model.Metric, t *time.Time, s *metric.StalenessPolicy) (*model.Sample, error) {
	panic("not implemented")
}

func (l *LevelDBMetricPersistence) GetRangeValues(m *model.Metric, i *model.Interval, s *metric.StalenessPolicy) (*model.SampleSet, error) {
	panic("not implemented")
}
