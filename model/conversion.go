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

package model

import (
	"code.google.com/p/goprotobuf/proto"
	"crypto/md5"
	"encoding/hex"
	data "github.com/matttproud/prometheus/model/generated"
	"io"
	"sort"
)

func SampleToMetricDDO(s *Sample) *data.MetricDDO {
	labelLength := len(s.Labels)
	labelNames := make([]string, 0, labelLength)

	for labelName := range s.Labels {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelPairs := make([]*data.LabelPairDDO, 0, labelLength)

	for _, labelName := range labelNames {
		labelValue := s.Labels[labelName]
		labelPair := &data.LabelPairDDO{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelPairs = append(labelPairs, labelPair)
	}

	return &data.MetricDDO{
		LabelPair: labelPairs,
	}
}

func MetricToMetricDDO(m *Metric) *data.MetricDDO {
	metricLength := len(*m)
	labelNames := make([]string, 0, metricLength)

	for labelName := range *m {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	labelPairs := make([]*data.LabelPairDDO, 0, metricLength)

	for _, labelName := range labelNames {
		labelValue := (*m)[labelName]
		labelPair := &data.LabelPairDDO{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		labelPairs = append(labelPairs, labelPair)
	}

	return &data.MetricDDO{
		LabelPair: labelPairs,
	}
}

func BytesToFingerprintDDO(b []byte) *data.FingerprintDDO {
	return &data.FingerprintDDO{
		Signature: proto.String(string(b)),
	}
}

func StringToFingerprint(v string) Fingerprint {
	hash := md5.New()
	io.WriteString(hash, v)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}

func BytesToFingerprint(v []byte) Fingerprint {
	hash := md5.New()
	hash.Write(v)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}
