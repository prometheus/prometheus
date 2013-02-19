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
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

func (l *LevelDBMetricPersistence) setLabelPairFingerprints(labelPair *dto.LabelPair, fingerprints *dto.FingerprintCollection) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: setLabelPairFingerprints, result: success}, map[string]string{operation: setLabelPairFingerprints, result: failure})
	}()

	labelPairEncoded := coding.NewProtocolBufferEncoder(labelPair)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)
	err = l.labelSetToFingerprints.Put(labelPairEncoded, fingerprintsEncoded)

	return
}

func (l *LevelDBMetricPersistence) setLabelNameFingerprints(labelName *dto.LabelName, fingerprints *dto.FingerprintCollection) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: setLabelNameFingerprints, result: success}, map[string]string{operation: setLabelNameFingerprints, result: failure})
	}()

	labelNameEncoded := coding.NewProtocolBufferEncoder(labelName)
	fingerprintsEncoded := coding.NewProtocolBufferEncoder(fingerprints)

	err = l.labelNameToFingerprints.Put(labelNameEncoded, fingerprintsEncoded)

	return
}

func (l *LevelDBMetricPersistence) appendLabelPairFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: appendLabelPairFingerprint, result: success}, map[string]string{operation: appendLabelPairFingerprint, result: failure})
	}()

	has, err := l.HasLabelPair(labelPair)
	if err != nil {
		return
	}

	var fingerprints *dto.FingerprintCollection
	if has {
		fingerprints, err = l.getFingerprintsForLabelSet(labelPair)
		if err != nil {
			return
		}
	} else {
		fingerprints = &dto.FingerprintCollection{}
	}

	fingerprints.Member = append(fingerprints.Member, fingerprint)

	err = l.setLabelPairFingerprints(labelPair, fingerprints)

	return
}

func (l *LevelDBMetricPersistence) appendLabelNameFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: appendLabelNameFingerprint, result: success}, map[string]string{operation: appendLabelNameFingerprint, result: failure})
	}()

	labelName := &dto.LabelName{
		Name: labelPair.Name,
	}

	has, err := l.HasLabelName(labelName)
	if err != nil {
		return
	}

	var fingerprints *dto.FingerprintCollection
	if has {
		fingerprints, err = l.GetLabelNameFingerprints(labelName)
		if err != nil {
			return
		}
	} else {
		fingerprints = &dto.FingerprintCollection{}
	}

	fingerprints.Member = append(fingerprints.Member, fingerprint)

	err = l.setLabelNameFingerprints(labelName, fingerprints)

	return
}

func (l *LevelDBMetricPersistence) appendFingerprints(sample model.Sample) (err error) {
	begin := time.Now()

	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: appendFingerprints, result: success}, map[string]string{operation: appendFingerprints, result: failure})
	}()

	fingerprintDTO := model.NewFingerprintFromMetric(sample.Metric).ToDTO()

	fingerprintKey := coding.NewProtocolBufferEncoder(fingerprintDTO)
	metricDTO := model.SampleToMetricDTO(&sample)
	metricDTOEncoder := coding.NewProtocolBufferEncoder(metricDTO)

	err = l.fingerprintToMetrics.Put(fingerprintKey, metricDTOEncoder)
	if err != nil {
		return
	}

	labelCount := len(metricDTO.LabelPair)
	labelPairErrors := make(chan error, labelCount)
	labelNameErrors := make(chan error, labelCount)

	for _, labelPair := range metricDTO.LabelPair {
		go func(labelPair *dto.LabelPair) {
			labelNameErrors <- l.appendLabelNameFingerprint(labelPair, fingerprintDTO)
		}(labelPair)

		go func(labelPair *dto.LabelPair) {
			labelPairErrors <- l.appendLabelPairFingerprint(labelPair, fingerprintDTO)
		}(labelPair)
	}

	for i := 0; i < cap(labelPairErrors); i++ {
		err = <-labelPairErrors

		if err != nil {
			return
		}
	}

	for i := 0; i < cap(labelNameErrors); i++ {
		err = <-labelNameErrors

		if err != nil {
			return
		}
	}

	return
}

func (l *LevelDBMetricPersistence) AppendSample(sample model.Sample) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Now().Sub(begin)

		recordOutcome(storageOperations, storageLatency, duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: appendSample, result: failure})
	}()

	metricDTO := model.SampleToMetricDTO(&sample)

	indexHas, err := l.hasIndexMetric(metricDTO)
	if err != nil {
		return
	}

	fingerprint := model.NewFingerprintFromMetric(sample.Metric)

	if !indexHas {
		err = l.indexMetric(metricDTO)
		if err != nil {
			return
		}

		err = l.appendFingerprints(sample)
		if err != nil {
			return
		}
	}

	fingerprintDTO := fingerprint.ToDTO()

	sampleKeyDTO := &dto.SampleKey{
		Fingerprint: fingerprintDTO,
		Timestamp:   indexable.EncodeTime(sample.Timestamp),
	}
	sampleValueDTO := &dto.SampleValueSeries{}
	sampleValueDTO.Value = append(sampleValueDTO.Value, &dto.SampleValueSeries_Value{
		Value: proto.Float32(float32(sample.Value)),
	})
	sampleKeyEncoded := coding.NewProtocolBufferEncoder(sampleKeyDTO)
	sampleValueEncoded := coding.NewProtocolBufferEncoder(sampleValueDTO)

	err = l.metricSamples.Put(sampleKeyEncoded, sampleValueEncoded)
	if err != nil {
		return
	}

	return
}
