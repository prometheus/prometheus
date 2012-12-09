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
	"log"
)

func (l *LevelDBMetricPersistence) setLabelPairFingerprints(labelPair *dto.LabelPair, fingerprints *dto.FingerprintCollection) error {
	labelPairEncoded := coding.NewProtocolBufferEncoder(labelPair)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelSetToFingerprints.Put(labelPairEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) setLabelNameFingerprints(labelName *dto.LabelName, fingerprints *dto.FingerprintCollection) error {
	labelNameEncoded := coding.NewProtocolBufferEncoder(labelName)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	return l.labelNameToFingerprints.Put(labelNameEncoded, fingerprintsEncoded)
}

func (l *LevelDBMetricPersistence) appendLabelPairFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) error {
	if has, hasError := l.HasLabelPair(labelPair); hasError == nil {
		var fingerprints *dto.FingerprintCollection
		if has {
			if existing, existingError := l.getFingerprintsForLabelSet(labelPair); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &dto.FingerprintCollection{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelPairFingerprints(labelPair, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendLabelNameFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) error {
	labelName := &dto.LabelName{
		Name: labelPair.Name,
	}

	if has, hasError := l.HasLabelName(labelName); hasError == nil {
		var fingerprints *dto.FingerprintCollection
		if has {
			if existing, existingError := l.GetLabelNameFingerprints(labelName); existingError == nil {
				fingerprints = existing
			} else {
				return existingError
			}
		} else {
			fingerprints = &dto.FingerprintCollection{}
		}

		fingerprints.Member = append(fingerprints.Member, fingerprint)

		return l.setLabelNameFingerprints(labelName, fingerprints)
	} else {
		return hasError
	}

	return errors.New("Unknown error when appending fingerprint to label name and value pair.")
}

func (l *LevelDBMetricPersistence) appendFingerprints(m *dto.Metric) error {
	if fingerprintDTO, fingerprintDTOError := model.MessageToFingerprintDTO(m); fingerprintDTOError == nil {
		fingerprintKey := coding.NewProtocolBufferEncoder(fingerprintDTO)
		metricDTOEncoder := coding.NewProtocolBufferEncoder(m)

		if putError := l.fingerprintToMetrics.Put(fingerprintKey, metricDTOEncoder); putError == nil {
			labelCount := len(m.LabelPair)
			labelPairErrors := make(chan error, labelCount)
			labelNameErrors := make(chan error, labelCount)

			for _, labelPair := range m.LabelPair {
				go func(labelPair *dto.LabelPair) {
					labelNameErrors <- l.appendLabelNameFingerprint(labelPair, fingerprintDTO)
				}(labelPair)

				go func(labelPair *dto.LabelPair) {
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

	if fingerprintDTO, fingerprintDTOErr := model.MessageToFingerprintDTO(metricDTO); fingerprintDTOErr == nil {

		sampleKeyDTO := &dto.SampleKey{
			Fingerprint: fingerprintDTO,
			Timestamp:   indexable.EncodeTime(sample.Timestamp),
		}

		sampleValueDTO := &dto.SampleValue{
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
