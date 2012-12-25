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
	registry "github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/golang_instrumentation/metrics"
	"github.com/matttproud/prometheus/coding"
	"github.com/matttproud/prometheus/coding/indexable"
	"github.com/matttproud/prometheus/model"
	dto "github.com/matttproud/prometheus/model/generated"
)

var (
	appendSuccessCount = &metrics.CounterMetric{}
	appendFailureCount = &metrics.CounterMetric{}
)

func init() {
	registry.Register("sample_append_success_count_total", appendSuccessCount)
	registry.Register("sample_append_failure_count_total", appendFailureCount)
}

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

func (l *LevelDBMetricPersistence) appendLabelPairFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) (err error) {
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

	return l.setLabelPairFingerprints(labelPair, fingerprints)
}

func (l *LevelDBMetricPersistence) appendLabelNameFingerprint(labelPair *dto.LabelPair, fingerprint *dto.Fingerprint) (err error) {
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

	return l.setLabelNameFingerprints(labelName, fingerprints)
}

func (l *LevelDBMetricPersistence) appendFingerprints(m *dto.Metric) (err error) {
	fingerprintDTO, err := model.MessageToFingerprintDTO(m)
	if err != nil {
		return
	}

	fingerprintKey := coding.NewProtocolBufferEncoder(fingerprintDTO)
	metricDTOEncoder := coding.NewProtocolBufferEncoder(m)

	err = l.fingerprintToMetrics.Put(fingerprintKey, metricDTOEncoder)
	if err != nil {
		return
	}

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

func (l *LevelDBMetricPersistence) AppendSample(sample *model.Sample) (err error) {
	defer func() {
		var m *metrics.CounterMetric = appendSuccessCount

		if err != nil {
			m = appendFailureCount
		}

		m.Increment()
	}()

	metricDTO := model.SampleToMetricDTO(sample)

	indexHas, err := l.hasIndexMetric(metricDTO)
	if err != nil {
		return
	}

	if !indexHas {
		err = l.indexMetric(metricDTO)
		if err != nil {
			return
		}

		err = l.appendFingerprints(metricDTO)
		if err != nil {
			return
		}
	}

	fingerprintDTO, err := model.MessageToFingerprintDTO(metricDTO)
	if err != nil {
		return
	}

	sampleKeyDTO := &dto.SampleKey{
		Fingerprint: fingerprintDTO,
		Timestamp:   indexable.EncodeTime(sample.Timestamp),
	}
	sampleValueDTO := &dto.SampleValue{
		Value: proto.Float32(float32(sample.Value)),
	}
	sampleKeyEncoded := coding.NewProtocolBufferEncoder(sampleKeyDTO)
	sampleValueEncoded := coding.NewProtocolBufferEncoder(sampleValueDTO)

	err = l.metricSamples.Put(sampleKeyEncoded, sampleValueEncoded)
	if err != nil {
		return
	}

	return
}
