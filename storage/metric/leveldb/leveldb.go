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
	fingerprintHighWaterMarks *storage.LevelDBPersistence
	fingerprintLabelPairs     *storage.LevelDBPersistence
	fingerprintLowWaterMarks  *storage.LevelDBPersistence
	fingerprintSamples        *storage.LevelDBPersistence
	labelNameFingerprints     *storage.LevelDBPersistence
	labelPairFingerprints     *storage.LevelDBPersistence
	metricMembershipIndex     *index.LevelDBMembershipIndex
}

type leveldbOpener func()

func (l *LevelDBMetricPersistence) Close() error {
	log.Printf("Closing LevelDBPersistence storage containers...")

	var persistences = []struct {
		name   string
		closer io.Closer
	}{
		{
			"Fingerprint High-Water Marks",
			l.fingerprintHighWaterMarks,
		},
		{
			"Fingerprint to Label Name and Value Pairs",
			l.fingerprintLabelPairs,
		},
		{
			"Fingerprint Low-Water Marks",
			l.fingerprintLowWaterMarks,
		},
		{
			"Fingerprint Samples",
			l.fingerprintSamples,
		},
		{
			"Label Name to Fingerprints",
			l.labelNameFingerprints,
		},
		{
			"Label Name and Value Pairs to Fingerprints",
			l.labelPairFingerprints,
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

	errorChannel := make(chan error, 7)

	emission := &LevelDBMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"High-Water Marks by Fingerprint",
			func() {
				var err error
				emission.fingerprintHighWaterMarks, err = storage.NewLevelDBPersistence(baseDirectory+"/high_water_marks_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.fingerprintLabelPairs, err = storage.NewLevelDBPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Low-Water Marks by Fingerprint",
			func() {
				var err error
				emission.fingerprintLowWaterMarks, err = storage.NewLevelDBPersistence(baseDirectory+"/low_water_marks_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.fingerprintSamples, err = storage.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelPairFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", 1000000, 10)
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

func (l *LevelDBMetricPersistence) hasIndexMetric(ddo *data.MetricDDO) (bool, error) {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	return l.metricMembershipIndex.Has(ddoKey)
}

func (l *LevelDBMetricPersistence) indexMetric(ddo *data.MetricDDO) error {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	return l.metricMembershipIndex.Put(ddoKey)
}

func fingerprintDDOForMessage(message proto.Message) (*data.FingerprintDDO, error) {
	if messageByteArray, marshalError := proto.Marshal(message); marshalError == nil {
		fingerprint := model.BytesToFingerprint(messageByteArray)
		return &data.FingerprintDDO{
			Signature: proto.String(string(fingerprint)),
		}, nil
	} else {
		return nil, marshalError
	}

	return nil, errors.New("Unknown error in generating FingerprintDDO from message.")
}

func (l *LevelDBMetricPersistence) HasLabelPair(ddo *data.LabelPairDDO) (bool, error) {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	return l.labelPairFingerprints.Has(ddoKey)
}

func (l *LevelDBMetricPersistence) HasLabelName(ddo *data.LabelNameDDO) (bool, error) {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	return l.labelNameFingerprints.Has(ddoKey)
}

func (l *LevelDBMetricPersistence) GetLabelPairFingerprints(ddo *data.LabelPairDDO) (*data.FingerprintCollectionDDO, error) {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	if get, getError := l.labelPairFingerprints.Get(ddoKey); getError == nil {
		value := &data.FingerprintCollectionDDO{}
		if unmarshalError := proto.Unmarshal(get, value); unmarshalError == nil {
			return value, nil
		} else {
			return nil, unmarshalError
		}
	} else {
		return nil, getError
	}
	return nil, errors.New("Unknown error while getting label name and value pair fingerprints.")
}

func (l *LevelDBMetricPersistence) GetLabelNameFingerprints(ddo *data.LabelNameDDO) (*data.FingerprintCollectionDDO, error) {
	ddoKey := coding.NewProtocolBufferEncoder(ddo)
	if get, getError := l.labelNameFingerprints.Get(ddoKey); getError == nil {
		value := &data.FingerprintCollectionDDO{}
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

func (l *LevelDBMetricPersistence) setLabelPairFingerprints(labelPair *data.LabelPairDDO, fingerprints *data.FingerprintCollectionDDO) error {
	labelPairEncoded := coding.NewProtocolBufferEncoder(labelPair)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelPairFingerprints.Put(labelPairEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) setLabelNameFingerprints(labelName *data.LabelNameDDO, fingerprints *data.FingerprintCollectionDDO) error {
	labelNameEncoded := coding.NewProtocolBufferEncoder(labelName)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelNameFingerprints.Put(labelNameEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) appendLabelPairFingerprint(labelPair *data.LabelPairDDO, fingerprint *data.FingerprintDDO) error {
	if has, hasError := l.HasLabelPair(labelPair); hasError == nil {
		var fingerprints *data.FingerprintCollectionDDO
		if has {
			if existing, existingError := l.GetLabelPairFingerprints(labelPair); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &data.FingerprintCollectionDDO{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelPairFingerprints(labelPair, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendLabelNameFingerprint(labelPair *data.LabelPairDDO, fingerprint *data.FingerprintDDO) error {
	labelName := &data.LabelNameDDO{
		Name: labelPair.Name,
	}

	if has, hasError := l.HasLabelName(labelName); hasError == nil {
		var fingerprints *data.FingerprintCollectionDDO
		if has {
			if existing, existingError := l.GetLabelNameFingerprints(labelName); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &data.FingerprintCollectionDDO{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelNameFingerprints(labelName, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendFingerprints(ddo *data.MetricDDO) error {
	if fingerprintDDO, fingerprintDDOError := fingerprintDDOForMessage(ddo); fingerprintDDOError == nil {
		labelPairCollectionDDO := &data.LabelPairCollectionDDO{
			Member: ddo.LabelPair,
		}
		fingerprintKey := coding.NewProtocolBufferEncoder(fingerprintDDO)
		labelPairCollectionDDOEncoder := coding.NewProtocolBufferEncoder(labelPairCollectionDDO)

		if putError := l.fingerprintLabelPairs.Put(fingerprintKey, labelPairCollectionDDOEncoder); putError == nil {
			labelCount := len(ddo.LabelPair)
			labelPairErrors := make(chan error, labelCount)
			labelNameErrors := make(chan error, labelCount)

			for _, labelPair := range ddo.LabelPair {
				go func(labelPair *data.LabelPairDDO) {
					labelNameErrors <- l.appendLabelNameFingerprint(labelPair, fingerprintDDO)
				}(labelPair)

				go func(labelPair *data.LabelPairDDO) {
					labelPairErrors <- l.appendLabelPairFingerprint(labelPair, fingerprintDDO)
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
		return fingerprintDDOError
	}

	return errors.New("Unknown error in appending label pairs to fingerprint.")
}

func (l *LevelDBMetricPersistence) AppendSample(sample *model.Sample) error {
	metricDDO := model.SampleToMetricDDO(sample)

	if indexHas, indexHasError := l.hasIndexMetric(metricDDO); indexHasError == nil {
		if !indexHas {
			if indexPutError := l.indexMetric(metricDDO); indexPutError == nil {
				if appendError := l.appendFingerprints(metricDDO); appendError != nil {
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

	if fingerprintDDO, fingerprintDDOErr := fingerprintDDOForMessage(metricDDO); fingerprintDDOErr == nil {

		sampleKeyDDO := &data.SampleKeyDDO{
			Fingerprint: fingerprintDDO,
			Timestamp:   indexable.EncodeTime(sample.Timestamp),
		}

		sampleValueDDO := &data.SampleValueDDO{
			Value: proto.Float32(float32(sample.Value)),
		}

		sampleKeyEncoded := coding.NewProtocolBufferEncoder(sampleKeyDDO)
		sampleValueEncoded := coding.NewProtocolBufferEncoder(sampleValueDDO)

		if putError := l.fingerprintSamples.Put(sampleKeyEncoded, sampleValueEncoded); putError != nil {
			log.Printf("Could not append metric sample: %q\n", putError)
			return putError
		}
	} else {
		log.Printf("Could not encode metric fingerprint: %q\n", fingerprintDDOErr)
		return fingerprintDDOErr
	}

	return nil
}

func (l *LevelDBMetricPersistence) GetLabelNames() ([]string, error) {
	if getAll, getAllError := l.labelNameFingerprints.GetAll(); getAllError == nil {
		result := make([]string, 0, len(getAll))
		labelNameDDO := &data.LabelNameDDO{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelNameDDO); unmarshalError == nil {
				result = append(result, *labelNameDDO.Name)
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

func (l *LevelDBMetricPersistence) GetLabelPairs() ([]model.LabelPairs, error) {
	if getAll, getAllError := l.labelPairFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelPairs, 0, len(getAll))
		labelPairDDO := &data.LabelPairDDO{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelPairDDO); unmarshalError == nil {
				item := model.LabelPairs{
					*labelPairDDO.Name: *labelPairDDO.Value,
				}
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

func (l *LevelDBMetricPersistence) GetMetrics() ([]model.LabelPairs, error) {
	if getAll, getAllError := l.labelPairFingerprints.GetAll(); getAllError == nil {
		result := make([]model.LabelPairs, 0)
		fingerprintCollection := &data.FingerprintCollectionDDO{}

		fingerprints := make(utility.Set)

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Right, fingerprintCollection); unmarshalError == nil {
				for _, member := range fingerprintCollection.Member {
					if !fingerprints.Has(*member.Signature) {
						fingerprints.Add(*member.Signature)
						fingerprintEncoded := coding.NewProtocolBufferEncoder(member)
						if labelPairCollectionRaw, labelPairCollectionRawError := l.fingerprintLabelPairs.Get(fingerprintEncoded); labelPairCollectionRawError == nil {

							labelPairCollectionDDO := &data.LabelPairCollectionDDO{}

							if labelPairCollectionDDOMarshalError := proto.Unmarshal(labelPairCollectionRaw, labelPairCollectionDDO); labelPairCollectionDDOMarshalError == nil {
								intermediate := make(model.LabelPairs, 0)

								for _, member := range labelPairCollectionDDO.Member {
									intermediate[*member.Name] = *member.Value
								}

								result = append(result, intermediate)
							} else {
								return nil, labelPairCollectionDDOMarshalError
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

func (l *LevelDBMetricPersistence) GetWatermarksForMetric(metric model.Metric) (*model.Interval, int, error) {
	metricDDO := model.MetricToMetricDDO(&metric)

	if fingerprintDDO, fingerprintDDOErr := fingerprintDDOForMessage(metricDDO); fingerprintDDOErr == nil {
		if iterator, closer, iteratorErr := l.fingerprintSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &data.SampleKeyDDO{
				Fingerprint: fingerprintDDO,
				Timestamp:   indexable.EarliestTime,
			}

			if encode, encodeErr := coding.NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				if iterator.Valid() {
					found := &data.SampleKeyDDO{}
					if unmarshalErr := proto.Unmarshal(iterator.Key(), found); unmarshalErr == nil {
						var foundEntries int = 0

						if *fingerprintDDO.Signature == *found.Fingerprint.Signature {
							emission := &model.Interval{
								OldestInclusive: indexable.DecodeTime(found.Timestamp),
								NewestInclusive: indexable.DecodeTime(found.Timestamp),
							}

							for iterator = iterator; iterator.Valid(); iterator.Next() {
								if subsequentUnmarshalErr := proto.Unmarshal(iterator.Key(), found); subsequentUnmarshalErr == nil {
									if *fingerprintDDO.Signature != *found.Fingerprint.Signature {
										return emission, foundEntries, nil
									}
									foundEntries++
									emission.NewestInclusive = indexable.DecodeTime(found.Timestamp)
								} else {
									log.Printf("Could not de-serialize subsequent key: %q\n", subsequentUnmarshalErr)
									return nil, -7, subsequentUnmarshalErr
								}
							}
							return emission, foundEntries, nil
						} else {
							return &model.Interval{}, -6, nil
						}
					} else {
						log.Printf("Could not de-serialize start key: %q\n", unmarshalErr)
						return nil, -5, unmarshalErr
					}
				} else {
					iteratorErr := iterator.GetError()
					log.Printf("Could not seek for metric watermark beginning: %q\n", iteratorErr)
					return nil, -4, iteratorErr
				}
			} else {
				log.Printf("Could not seek for metric watermark: %q\n", encodeErr)
				return nil, -3, encodeErr
			}
		} else {
			if closer != nil {
				defer closer.Close()
			}

			log.Printf("Could not provision iterator for metric: %q\n", iteratorErr)
			return nil, -3, iteratorErr
		}
	} else {
		log.Printf("Could not encode metric: %q\n", fingerprintDDOErr)
		return nil, -2, fingerprintDDOErr
	}

	return nil, -1, errors.New("Unknown error occurred while querying metric watermarks.")
}

func (l *LevelDBMetricPersistence) GetSamplesForMetric(metric model.Metric, interval model.Interval) ([]model.Samples, error) {
	metricDDO := model.MetricToMetricDDO(&metric)

	if fingerprintDDO, fingerprintDDOErr := fingerprintDDOForMessage(metricDDO); fingerprintDDOErr == nil {
		if iterator, closer, iteratorErr := l.fingerprintSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &data.SampleKeyDDO{
				Fingerprint: fingerprintDDO,
				Timestamp:   indexable.EncodeTime(interval.OldestInclusive),
			}

			emission := make([]model.Samples, 0)

			if encode, encodeErr := coding.NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				for iterator = iterator; iterator.Valid(); iterator.Next() {
					key := &data.SampleKeyDDO{}
					value := &data.SampleValueDDO{}
					if keyUnmarshalErr := proto.Unmarshal(iterator.Key(), key); keyUnmarshalErr == nil {
						if valueUnmarshalErr := proto.Unmarshal(iterator.Value(), value); valueUnmarshalErr == nil {
							if *fingerprintDDO.Signature == *key.Fingerprint.Signature {
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
		log.Printf("Could not create fingerprint for the metric: %q\n", fingerprintDDOErr)
		return nil, fingerprintDDOErr
	}

	return nil, errors.New("Unknown error occurred while querying metric watermarks.")
}

func (l *LevelDBMetricPersistence) GetFingerprintLabelPairs(f model.Fingerprint) (model.LabelPairs, error) {
	panic("NOT IMPLEMENTED")
}

func (l *LevelDBMetricPersistence) GetMetricFingerprintsForLabelPairs(p []*model.LabelPairs) ([]*model.Fingerprint, error) {
	panic("NOT IMPLEMENTED")
}

func (l *LevelDBMetricPersistence) RecordFingerprintWatermark(s *model.Sample) error {
	panic("NOT IMPLEMENTED")
}
