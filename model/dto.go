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

package model

import (
	"code.google.com/p/goprotobuf/proto"
	dto "github.com/prometheus/prometheus/model/generated"
	"sort"
	"time"
)

func SampleToMetricDTO(s *Sample) *dto.Metric {
	labelLength := len(s.Metric)
	labelNames := make([]string, 0, labelLength)

	for labelName := range s.Metric {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelSets := make([]*dto.LabelPair, 0, labelLength)

	for _, labelName := range labelNames {
		labelValue := s.Metric[LabelName(labelName)]
		labelPair := &dto.LabelPair{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelSets = append(labelSets, labelPair)
	}

	return &dto.Metric{
		LabelPair: labelSets,
	}
}

func MetricToDTO(m Metric) *dto.Metric {
	metricLength := len(m)
	labelNames := make([]string, 0, metricLength)

	for labelName := range m {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelSets := make([]*dto.LabelPair, 0, metricLength)

	for _, labelName := range labelNames {
		l := LabelName(labelName)
		labelValue := m[l]
		labelPair := &dto.LabelPair{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelSets = append(labelSets, labelPair)
	}

	return &dto.Metric{
		LabelPair: labelSets,
	}
}

func LabelSetToDTOs(s *LabelSet) []*dto.LabelPair {
	metricLength := len(*s)
	labelNames := make([]string, 0, metricLength)

	for labelName := range *s {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelSets := make([]*dto.LabelPair, 0, metricLength)

	for _, labelName := range labelNames {
		l := LabelName(labelName)
		labelValue := (*s)[l]
		labelPair := &dto.LabelPair{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelSets = append(labelSets, labelPair)
	}

	return labelSets
}

func LabelSetToDTO(s *LabelSet) *dto.LabelSet {
	return &dto.LabelSet{
		Member: LabelSetToDTOs(s),
	}
}

func LabelNameToDTO(l *LabelName) *dto.LabelName {
	return &dto.LabelName{
		Name: proto.String(string(*l)),
	}
}

func FingerprintToDTO(f Fingerprint) *dto.Fingerprint {
	return &dto.Fingerprint{
		Signature: proto.String(f.ToRowKey()),
	}
}

func SampleFromDTO(m *Metric, t *time.Time, v *dto.SampleValueSeries) *Sample {
	s := &Sample{
		Value:     SampleValue(*v.Value[0].Value),
		Timestamp: *t,
	}

	s.Metric = *m

	return s
}
