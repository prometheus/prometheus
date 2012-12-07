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
	data "github.com/matttproud/prometheus/model/generated"
	index "github.com/matttproud/prometheus/storage/raw/index/leveldb"
	storage "github.com/matttproud/prometheus/storage/raw/leveldb"
	"github.com/matttproud/prometheus/utility"
	"io"
	"log"
)

type LevelDBMetricPersistence struct {
	fingerprintToMetrics    *storage.LevelDBPersistence
	metricSamples           *storage.LevelDBPersistence
	labelNameToFingerprints *storage.LevelDBPersistence
	labelSetToFingerprints  *storage.LevelDBPersistence
	metricMembershipIndex   *index.LevelDBMembershipIndex
}

type leveldbOpener func()

func (l *LevelDBMetricPersistence) Close() error {
	log.Printf("Closing LevelDBPersistence storage containers...")

	var persistences = []struct {
		name   string
		closer io.Closer
	}{
		{
			"Fingerprint to Label Name and Value Pairs",
			l.fingerprintToMetrics,
		},
		{
			"Fingerprint Samples",
			l.metricSamples,
		},
		{
			"Label Name to Fingerprints",
			l.labelNameToFingerprints,
		},
		{
			"Label Name and Value Pairs to Fingerprints",
			l.labelSetToFingerprints,
		},
		{
			"Metric Membership Index",
			l.metricMembershipIndex,
		},
	}

	errorChannel := make(chan error, len(persistences))

	for _, persistence := range persistences {
		name := persistence.name
		closer := persistence.closer

		go func(name string, closer io.Closer) {
			if closer != nil {
				log.Printf("Closing LevelDBPersistence storage container: %s\n", name)
				closingError := closer.Close()

				if closingError != nil {
					log.Printf("Could not close a LevelDBPersistence storage container; inconsistencies are possible: %q\n", closingError)
				}

				errorChannel <- closingError
			} else {
				errorChannel <- nil
			}
		}(name, closer)
	}

	for i := 0; i < cap(errorChannel); i++ {
		closingError := <-errorChannel

		if closingError != nil {
			return closingError
		}
	}

	log.Printf("Successfully closed all LevelDBPersistence storage containers.")

	return nil
}

func NewLevelDBMetricPersistence(baseDirectory string) (*LevelDBMetricPersistence, error) {
	log.Printf("Opening LevelDBPersistence storage containers...")

	errorChannel := make(chan error, 5)

	emission := &LevelDBMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.fingerprintToMetrics, err = storage.NewLevelDBPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.metricSamples, err = storage.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelSetToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.metricMembershipIndex, err = index.NewLevelDBMembershipIndex(baseDirectory+"/metric_membership_index", 1000000, 10)
				errorChannel <- err
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		name := subsystem.name
		opener := subsystem.opener

		log.Printf("Opening LevelDBPersistence storage container: %s\n", name)

		go opener()
	}

	for i := 0; i < cap(errorChannel); i++ {
		openingError := <-errorChannel

		if openingError != nil {
			log.Printf("Could not open a LevelDBPersistence storage container: %q\n", openingError)

			return nil, openingError
		}
	}

	log.Printf("Successfully opened all LevelDBPersistence storage containers.\n")

	return emission, nil
}

func (l *LevelDBMetricPersistence) hasIndexMetric(dto *data.Metric) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.metricMembershipIndex.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) indexMetric(dto *data.Metric) error {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.metricMembershipIndex.Put(dtoKey)
}

// TODO(mtp): Candidate for refactoring.
func fingerprintDTOForMessage(message proto.Message) (*data.Fingerprint, error) {
	if messageByteArray, marshalError := proto.Marshal(message); marshalError == nil {
		fingerprint := model.BytesToFingerprint(messageByteArray)
		return &data.Fingerprint{
			Signature: proto.String(string(fingerprint)),
		}, nil
	} else {
		return nil, marshalError
	}

	return nil, errors.New("Unknown error in generating FingerprintDTO from message.")
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *data.LabelPair) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.labelSetToFingerprints.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *data.LabelName) (bool, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	return l.labelNameToFingerprints.Has(dtoKey)
}

func (l *LevelDBMetricPersistence) getFingerprintsForLabelSet(dto *data.LabelPair) (*data.FingerprintCollection, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	if get, getError := l.labelSetToFingerprints.Get(dtoKey); getError == nil {
		value := &data.FingerprintCollection{}
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

func (l *LevelDBMetricPersistence) GetLabelNameFingerprints(dto *data.LabelName) (*data.FingerprintCollection, error) {
	dtoKey := coding.NewProtocolBufferEncoder(dto)
	if get, getError := l.labelNameToFingerprints.Get(dtoKey); getError == nil {
		value := &data.FingerprintCollection{}
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

func (l *LevelDBMetricPersistence) setLabelPairFingerprints(labelPair *data.LabelPair, fingerprints *data.FingerprintCollection) error {
	labelPairEncoded := coding.NewProtocolBufferEncoder(labelPair)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelSetToFingerprints.Put(labelPairEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) setLabelNameFingerprints(labelName *data.LabelName, fingerprints *data.FingerprintCollection) error {
	labelNameEncoded := coding.NewProtocolBufferEncoder(labelName)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelNameToFingerprints.Put(labelNameEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) appendLabelPairFingerprint(labelPair *data.LabelPair, fingerprint *data.Fingerprint) error {
	if has, hasError := l.HasLabelPair(labelPair); hasError == nil {
		var fingerprints *data.FingerprintCollection
		if has {
			if existing, existingError := l.getFingerprintsForLabelSet(labelPair); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &data.FingerprintCollection{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelPairFingerprints(labelPair, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendLabelNameFingerprint(labelPair *data.LabelPair, fingerprint *data.Fingerprint) error {
	labelName := &data.LabelName{
		Name: labelPair.Name,
	}

	if has, hasError := l.HasLabelName(labelName); hasError == nil {
		var fingerprints *data.FingerprintCollection
		if has {
			if existing, existingError := l.GetLabelNameFingerprints(labelName); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &data.FingerprintCollection{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelNameFingerprints(labelName, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendFingerprints(dto *data.Metric) error {
	if fingerprintDTO, fingerprintDTOError := fingerprintDTOForMessage(dto); fingerprintDTOError == nil {
		fingerprintKey := coding.NewProtocolBufferEncoder(fingerprintDTO)
		metricDTOEncoder := coding.NewProtocolBufferEncoder(dto)

		if putError := l.fingerprintToMetrics.Put(fingerprintKey, metricDTOEncoder); putError == nil {
			labelCount := len(dto.LabelPair)
			labelPairErrors := make(chan error, labelCount)
			labelNameErrors := make(chan error, labelCount)

			for _, labelPair := range dto.LabelPair {
				go func(labelPair *data.LabelPair) {
					labelNameErrors <- l.appendLabelNameFingerprint(labelPair, fingerprintDTO)
				}(labelPair)

				go func(labelPair *data.LabelPair) {
					labelPairErrors <- l.appendLabelPairFingerprint(labelPair, fingerprintDTO)
				}(labelPair)
			}

			for i := 0; i < cap(labelPairErrors); i++ {
				appendError := <-labelPairErrors

				if appendError != nil {
					return appendError
				}
			}

			for i := 0; i < cap(labelNameErrors); i++ {
				appendError := <-labelNameErrors

				if appendError != nil {
					return appendError
				}
			}

			return nil

		} else {
			return putError
		}
	} else {
		return fingerprintDTOError
	}

	return errors.New("Unknown error in appending label pairs to fingerprint.")
}

func (l *LevelDBMetricPersistence) AppendSample(sample *model.Sample) error {
	metricDTO := model.SampleToMetricDTO(sample)

	if indexHas, indexHasError := l.hasIndexMetric(metricDTO); indexHasError == nil {
		if !indexHas {
			if indexPutError := l.indexMetric(metricDTO); indexPutError == nil {
				if appendError := l.appendFingerprints(metricDTO); appendError != nil {
					log.Printf("Could not set metric fingerprint to label pairs mapping: %q\n", appendError)
					return appendError
				}
			} else {
				log.Printf("Could not add metric to membership index: %q\n", indexPutError)
				return indexPutError
			}
		}
	} else {
		log.Printf("Could not query membership index for metric: %q\n", indexHasError)
		return indexHasError
	}

	if fingerprintDTO, fingerprintDTOErr := fingerprintDTOForMessage(metricDTO); fingerprintDTOErr == nil {

		sampleKeyDTO := &data.SampleKey{
			Fingerprint: fingerprintDTO,
			Timestamp:   indexable.EncodeTime(sample.Timestamp),
		}

		sampleValueDTO := &data.SampleValue{
			Value: proto.Float32(float32(sample.Value)),
		}

		sampleKeyEncoded := coding.NewProtocolBufferEncoder(sampleKeyDTO)
		sampleValueEncoded := coding.NewProtocolBufferEncoder(sampleValueDTO)

		if putError := l.metricSamples.Put(sampleKeyEncoded, sampleValueEncoded); putError != nil {
			log.Printf("Could not append metric sample: %q\n", putError)
			return putError
		}
	} else {
		log.Printf("Could not encode metric fingerprint: %q\n", fingerprintDTOErr)
		return fingerprintDTOErr
	}

	return nil
}

func (l *LevelDBMetricPersistence) GetAllLabelNames() ([]string, error) {
	if getAll, getAllError := l.labelNameToFingerprints.GetAll(); getAllError == nil {
		result := make([]string, 0, len(getAll))
		labelNameDTO := &data.LabelName{}

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
		labelPairDTO := &data.LabelPair{}

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
		fingerprintCollection := &data.FingerprintCollection{}

		fingerprints := make(utility.Set)

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Right, fingerprintCollection); unmarshalError == nil {
				for _, member := range fingerprintCollection.Member {
					if !fingerprints.Has(*member.Signature) {
						fingerprints.Add(*member.Signature)
						fingerprintEncoded := coding.NewProtocolBufferEncoder(member)
						if labelPairCollectionRaw, labelPairCollectionRawError := l.fingerprintToMetrics.Get(fingerprintEncoded); labelPairCollectionRawError == nil {

							labelPairCollectionDTO := &data.LabelSet{}

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
	metricDTO := model.MetricToDTO(&metric)

	if fingerprintDTO, fingerprintDTOErr := fingerprintDTOForMessage(metricDTO); fingerprintDTOErr == nil {
		if iterator, closer, iteratorErr := l.metricSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &data.SampleKey{
				Fingerprint: fingerprintDTO,
				Timestamp:   indexable.EncodeTime(interval.OldestInclusive),
			}

			emission := make([]model.Samples, 0)

			if encode, encodeErr := coding.NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				for iterator = iterator; iterator.Valid(); iterator.Next() {
					key := &data.SampleKey{}
					value := &data.SampleValue{}
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
			unmarshaled := &data.FingerprintCollection{}
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

		unmarshaled := &data.FingerprintCollection{}

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
