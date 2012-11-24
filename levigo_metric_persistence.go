package main

import (
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/coding/indexable"
	data "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/utility"
	"log"
	"sort"
)

type pendingArchival map[int64]float64

type LevigoMetricPersistence struct {
	fingerprintHighWaterMarks *LevigoPersistence
	fingerprintLabelPairs     *LevigoPersistence
	fingerprintLowWaterMarks  *LevigoPersistence
	fingerprintSamples        *LevigoPersistence
	labelNameFingerprints     *LevigoPersistence
	labelPairFingerprints     *LevigoPersistence
	metricMembershipIndex     *LevigoMembershipIndex
}

func (l *LevigoMetricPersistence) Close() error {
	log.Printf("Closing LevigoPersistence storage containers...")

	var persistences = []struct {
		name   string
		closer LevigoCloser
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

		if closer != nil {
			log.Printf("Closing LevigoPersistence storage container: %s\n", name)
			closingError := closer.Close()

			if closingError != nil {
				log.Printf("Could not close a LevigoPersistence storage container; inconsistencies are possible: %q\n", closingError)
			}

			errorChannel <- closingError
		} else {
			errorChannel <- nil
		}
	}

	for i := 0; i < cap(errorChannel); i++ {
		closingError := <-errorChannel

		if closingError != nil {
			return closingError
		}
	}

	log.Printf("Successfully closed all LevigoPersistence storage containers.")

	return nil
}

type levigoOpener func()

func NewLevigoMetricPersistence(baseDirectory string) (*LevigoMetricPersistence, error) {
	log.Printf("Opening LevigoPersistence storage containers...")

	errorChannel := make(chan error, 7)

	emission := &LevigoMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener levigoOpener
	}{
		{
			"High-Water Marks by Fingerprint",
			func() {
				var anomaly error
				emission.fingerprintHighWaterMarks, anomaly = NewLevigoPersistence(baseDirectory+"/high_water_marks_by_fingerprint", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var anomaly error
				emission.fingerprintLabelPairs, anomaly = NewLevigoPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Low-Water Marks by Fingerprint",
			func() {
				var anomaly error
				emission.fingerprintLowWaterMarks, anomaly = NewLevigoPersistence(baseDirectory+"/low_water_marks_by_fingerprint", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var anomaly error
				emission.fingerprintSamples, anomaly = NewLevigoPersistence(baseDirectory+"/samples_by_fingerprint", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var anomaly error
				emission.labelNameFingerprints, anomaly = NewLevigoPersistence(baseDirectory+"/fingerprints_by_label_name", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var anomaly error
				emission.labelPairFingerprints, anomaly = NewLevigoPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", 1000000, 10)
				errorChannel <- anomaly
			},
		},
		{
			"Metric Membership Index",
			func() {
				var anomaly error
				emission.metricMembershipIndex, anomaly = NewLevigoMembershipIndex(baseDirectory+"/metric_membership_index", 1000000, 10)
				errorChannel <- anomaly
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		name := subsystem.name
		opener := subsystem.opener

		log.Printf("Opening LevigoPersistence storage container: %s\n", name)

		go opener()
	}

	for i := 0; i < cap(errorChannel); i++ {
		openingError := <-errorChannel

		if openingError != nil {

			log.Printf("Could not open a LevigoPersistence storage container: %q\n", openingError)

			return nil, openingError
		}
	}

	log.Printf("Successfully opened all LevigoPersistence storage containers.\n")

	return emission, nil
}

func ddoFromSample(sample *Sample) *data.MetricDDO {
	labelNames := make([]string, 0, len(sample.Labels))

	for labelName, _ := range sample.Labels {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelPairs := make([]*data.LabelPairDDO, 0, len(sample.Labels))

	for _, labelName := range labelNames {
		labelValue := sample.Labels[labelName]
		labelPair := &data.LabelPairDDO{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelPairs = append(labelPairs, labelPair)
	}

	metricDDO := &data.MetricDDO{
		LabelPair: labelPairs,
	}

	return metricDDO
}

func ddoFromMetric(metric Metric) *data.MetricDDO {
	labelNames := make([]string, 0, len(metric))

	for labelName, _ := range metric {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelPairs := make([]*data.LabelPairDDO, 0, len(metric))

	for _, labelName := range labelNames {
		labelValue := metric[labelName]
		labelPair := &data.LabelPairDDO{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelPairs = append(labelPairs, labelPair)
	}

	metricDDO := &data.MetricDDO{
		LabelPair: labelPairs,
	}

	return metricDDO
}

func fingerprintDDOFromByteArray(fingerprint []byte) *data.FingerprintDDO {
	fingerprintDDO := &data.FingerprintDDO{
		Signature: proto.String(string(fingerprint)),
	}

	return fingerprintDDO
}

func (l *LevigoMetricPersistence) hasIndexMetric(ddo *data.MetricDDO) (bool, error) {
	ddoKey := NewProtocolBufferEncoder(ddo)
	return l.metricMembershipIndex.Has(ddoKey)
}

func (l *LevigoMetricPersistence) indexMetric(ddo *data.MetricDDO) error {
	ddoKey := NewProtocolBufferEncoder(ddo)
	return l.metricMembershipIndex.Put(ddoKey)
}

func fingerprintDDOForMessage(message proto.Message) (*data.FingerprintDDO, error) {
	if messageByteArray, marshalError := proto.Marshal(message); marshalError == nil {
		fingerprint := FingerprintFromByteArray(messageByteArray)
		return &data.FingerprintDDO{
			Signature: proto.String(string(fingerprint)),
		}, nil
	} else {
		return nil, marshalError
	}

	return nil, errors.New("Unknown error in generating FingerprintDDO from message.")
}

func (l *LevigoMetricPersistence) HasLabelPair(ddo *data.LabelPairDDO) (bool, error) {
	ddoKey := NewProtocolBufferEncoder(ddo)
	return l.labelPairFingerprints.Has(ddoKey)
}

func (l *LevigoMetricPersistence) HasLabelName(ddo *data.LabelNameDDO) (bool, error) {
	ddoKey := NewProtocolBufferEncoder(ddo)
	return l.labelNameFingerprints.Has(ddoKey)
}

func (l *LevigoMetricPersistence) GetLabelPairFingerprints(ddo *data.LabelPairDDO) (*data.FingerprintCollectionDDO, error) {
	ddoKey := NewProtocolBufferEncoder(ddo)
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

func (l *LevigoMetricPersistence) GetLabelNameFingerprints(ddo *data.LabelNameDDO) (*data.FingerprintCollectionDDO, error) {
	ddoKey := NewProtocolBufferEncoder(ddo)
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

func (l *LevigoMetricPersistence) setLabelPairFingerprints(labelPair *data.LabelPairDDO, fingerprints *data.FingerprintCollectionDDO) error {
	labelPairEncoded := NewProtocolBufferEncoder(labelPair)
	fingerprintsEncoded := NewProtocolBufferEncoder(fingerprints)
	return l.labelPairFingerprints.Put(labelPairEncoded, fingerprintsEncoded)
}

func (l *LevigoMetricPersistence) setLabelNameFingerprints(labelName *data.LabelNameDDO, fingerprints *data.FingerprintCollectionDDO) error {
	labelNameEncoded := NewProtocolBufferEncoder(labelName)
	fingerprintsEncoded := NewProtocolBufferEncoder(fingerprints)
	return l.labelNameFingerprints.Put(labelNameEncoded, fingerprintsEncoded)
}

func (l *LevigoMetricPersistence) appendLabelPairFingerprint(labelPair *data.LabelPairDDO, fingerprint *data.FingerprintDDO) error {
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

func (l *LevigoMetricPersistence) appendLabelNameFingerprint(labelPair *data.LabelPairDDO, fingerprint *data.FingerprintDDO) error {
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

func (l *LevigoMetricPersistence) appendFingerprints(ddo *data.MetricDDO) error {
	if fingerprintDDO, fingerprintDDOError := fingerprintDDOForMessage(ddo); fingerprintDDOError == nil {
		labelPairCollectionDDO := &data.LabelPairCollectionDDO{
			Member: ddo.LabelPair,
		}
		fingerprintKey := NewProtocolBufferEncoder(fingerprintDDO)
		labelPairCollectionDDOEncoder := NewProtocolBufferEncoder(labelPairCollectionDDO)

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

func (l *LevigoMetricPersistence) AppendSample(sample *Sample) error {
	fmt.Printf("Sample: %q\n", sample)

	metricDDO := ddoFromSample(sample)

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

		sampleKeyEncoded := NewProtocolBufferEncoder(sampleKeyDDO)
		sampleValueEncoded := NewProtocolBufferEncoder(sampleValueDDO)

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

func (l *LevigoMetricPersistence) GetLabelNames() ([]string, error) {
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

func (l *LevigoMetricPersistence) GetLabelPairs() ([]LabelPairs, error) {
	if getAll, getAllError := l.labelPairFingerprints.GetAll(); getAllError == nil {
		result := make([]LabelPairs, 0, len(getAll))
		labelPairDDO := &data.LabelPairDDO{}

		for _, pair := range getAll {
			if unmarshalError := proto.Unmarshal(pair.Left, labelPairDDO); unmarshalError == nil {
				item := LabelPairs{
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

func (l *LevigoMetricPersistence) GetMetrics() ([]LabelPairs, error) {
	log.Printf("GetMetrics()\n")

	if getAll, getAllError := l.labelPairFingerprints.GetAll(); getAllError == nil {
		log.Printf("getAll: %q\n", getAll)
		result := make([]LabelPairs, 0)
		fingerprintCollection := &data.FingerprintCollectionDDO{}

		fingerprints := make(utility.Set)

		for _, pair := range getAll {
			log.Printf("pair: %q\n", pair)
			if unmarshalError := proto.Unmarshal(pair.Right, fingerprintCollection); unmarshalError == nil {
				for _, member := range fingerprintCollection.Member {
					log.Printf("member: %q\n", member)
					if !fingerprints.Has(*member.Signature) {
						log.Printf("!Has: %q\n", member.Signature)
						fingerprints.Add(*member.Signature)
						log.Printf("fingerprints %q\n", fingerprints)
						fingerprintEncoded := NewProtocolBufferEncoder(member)
						if labelPairCollectionRaw, labelPairCollectionRawError := l.fingerprintLabelPairs.Get(fingerprintEncoded); labelPairCollectionRawError == nil {
							log.Printf("labelPairCollectionRaw: %q\n", labelPairCollectionRaw)

							labelPairCollectionDDO := &data.LabelPairCollectionDDO{}

							if labelPairCollectionDDOMarshalError := proto.Unmarshal(labelPairCollectionRaw, labelPairCollectionDDO); labelPairCollectionDDOMarshalError == nil {
								intermediate := make(LabelPairs, 0)

								for _, member := range labelPairCollectionDDO.Member {
									intermediate[*member.Name] = *member.Value
								}

								log.Printf("intermediate: %q\n", intermediate)

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

func (l *LevigoMetricPersistence) GetWatermarksForMetric(metric Metric) (*Interval, int, error) {
	metricDDO := ddoFromMetric(metric)

	if fingerprintDDO, fingerprintDDOErr := fingerprintDDOForMessage(metricDDO); fingerprintDDOErr == nil {
		if iterator, closer, iteratorErr := l.fingerprintSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &data.SampleKeyDDO{
				Fingerprint: fingerprintDDO,
				Timestamp:   indexable.EarliestTime,
			}

			if encode, encodeErr := NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				if iterator.Valid() {
					found := &data.SampleKeyDDO{}
					if unmarshalErr := proto.Unmarshal(iterator.Key(), found); unmarshalErr == nil {
						var foundEntries int = 0

						if *fingerprintDDO.Signature == *found.Fingerprint.Signature {
							emission := &Interval{
								OldestInclusive: indexable.DecodeTime(found.Timestamp),
								NewestInclusive: indexable.DecodeTime(found.Timestamp),
							}

							for iterator = iterator; iterator.Valid(); iterator.Next() {
								if subsequentUnmarshalErr := proto.Unmarshal(iterator.Key(), found); subsequentUnmarshalErr == nil {
									if *fingerprintDDO.Signature != *found.Fingerprint.Signature {
										return emission, foundEntries, nil
									}
									foundEntries++
									log.Printf("b foundEntries++ %d\n", foundEntries)
									emission.NewestInclusive = indexable.DecodeTime(found.Timestamp)
								} else {
									log.Printf("Could not de-serialize subsequent key: %q\n", subsequentUnmarshalErr)
									return nil, -7, subsequentUnmarshalErr
								}
							}
							return emission, foundEntries, nil
						} else {
							return &Interval{}, -6, nil
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

// TODO(mtp): Holes in the data!

func (l *LevigoMetricPersistence) GetSamplesForMetric(metric Metric, interval Interval) ([]Samples, error) {
	metricDDO := ddoFromMetric(metric)

	if fingerprintDDO, fingerprintDDOErr := fingerprintDDOForMessage(metricDDO); fingerprintDDOErr == nil {
		if iterator, closer, iteratorErr := l.fingerprintSamples.GetIterator(); iteratorErr == nil {
			defer closer.Close()

			start := &data.SampleKeyDDO{
				Fingerprint: fingerprintDDO,
				Timestamp:   indexable.EncodeTime(interval.OldestInclusive),
			}

			emission := make([]Samples, 0)

			if encode, encodeErr := NewProtocolBufferEncoder(start).Encode(); encodeErr == nil {
				iterator.Seek(encode)

				for iterator = iterator; iterator.Valid(); iterator.Next() {
					key := &data.SampleKeyDDO{}
					value := &data.SampleValueDDO{}
					if keyUnmarshalErr := proto.Unmarshal(iterator.Key(), key); keyUnmarshalErr == nil {
						if valueUnmarshalErr := proto.Unmarshal(iterator.Value(), value); valueUnmarshalErr == nil {
							if *fingerprintDDO.Signature == *key.Fingerprint.Signature {
								// Wart
								if indexable.DecodeTime(key.Timestamp).Unix() <= interval.NewestInclusive.Unix() {
									emission = append(emission, Samples{
										Value:     SampleValue(*value.Value),
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
