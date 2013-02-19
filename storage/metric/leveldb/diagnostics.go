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

package leveldb

import (
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/utility"
	"log"
)

func (l *LevelDBMetricPersistence) GetAllLabelNames() ([]string, error) {
	if getAll, getAllError := l.labelNameToFingerprints.GetAll(); getAllError == nil {
		result := make([]string, 0, len(getAll))
		labelNameDTO := &dto.LabelName{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelNameDTO); unmarshalError == nil {
				result = append(result, *labelNameDTO.Name)
			} else {
				return nil, unmarshalError
			}
		}

		return result, nil

	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying label names.")
}

func (l *LevelDBMetricPersistence) GetAllLabelPairs() ([]model.LabelSet, error) {
	if getAll, getAllError := l.labelSetToFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelSet, 0, len(getAll))
		labelPairDTO := &dto.LabelPair{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelPairDTO); unmarshalError == nil {
				n := model.LabelName(*labelPairDTO.Name)
				v := model.LabelValue(*labelPairDTO.Value)
				item := model.LabelSet{n: v}
				result = append(result, item)
			} else {
				return nil, unmarshalError
			}
		}

		return result, nil

	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying label pairs.")
}

func (l *LevelDBMetricPersistence) GetAllMetrics() ([]model.LabelSet, error) {
	if getAll, getAllError := l.labelSetToFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelSet, 0)
		fingerprintCollection := &dto.FingerprintCollection{}

		fingerprints := make(utility.Set)

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Right, fingerprintCollection); unmarshalError == nil {
				for _, member := range fingerprintCollection.Member {
					if !fingerprints.Has(*member.Signature) {
						fingerprints.Add(*member.Signature)
						fingerprintEncoded := coding.NewProtocolBufferEncoder(member)
						if labelPairCollectionRaw, labelPairCollectionRawError := l.fingerprintToMetrics.Get(fingerprintEncoded); labelPairCollectionRawError == nil {

							labelPairCollectionDTO := &dto.LabelSet{}

							if labelPairCollectionDTOMarshalError := proto.Unmarshal(labelPairCollectionRaw, labelPairCollectionDTO); labelPairCollectionDTOMarshalError == nil {
								intermediate := make(model.LabelSet, 0)

								for _, member := range labelPairCollectionDTO.Member {
									n := model.LabelName(*member.Name)
									v := model.LabelValue(*member.Value)
									intermediate[n] = v
								}

								result = append(result, intermediate)
							} else {
								return nil, labelPairCollectionDTOMarshalError
							}
						} else {
							return nil, labelPairCollectionRawError
						}
					}
				}
			} else {
				return nil, unmarshalError
			}
		}
		return result, nil
	} else {
		return nil, getAllError
	}

	return nil, errors.New("Unknown error encountered when querying metrics.")
}

func (l *LevelDBMetricPersistence) GetSamplesForMetric(metric model.Metric, interval model.Interval) ([]model.Samples, error) {
	if iterator, closer, iteratorErr := l.metricSamples.GetIterator(); iteratorErr == nil {
		defer closer.Close()

		fingerprintDTO := metric.Fingerprint().ToDTO()
		start := &dto.SampleKey{
			Fingerprint: fingerprintDTO,
			Timestamp:   indexable.EncodeTime(interval.OldestInclusive),
		}

		emission := make([]model.Samples, 0)

		if encode, encodeErr := coding.NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
			iterator.Seek(encode)

			predicate := keyIsAtMostOld(interval.NewestInclusive)

			for iterator = iterator; iterator.Valid(); iterator.Next() {
				key := &dto.SampleKey{}
				value := &dto.SampleValueSeries{}
				if keyUnmarshalErr := proto.Unmarshal(iterator.Key(), key); keyUnmarshalErr == nil {
					if valueUnmarshalErr := proto.Unmarshal(iterator.Value(), value); valueUnmarshalErr == nil {
						if fingerprintsEqual(fingerprintDTO, key.Fingerprint) {
							// Wart
							if predicate(key) {
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

	panic("unreachable")
}
