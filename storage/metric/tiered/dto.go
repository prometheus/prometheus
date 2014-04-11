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

package tiered

import (
	"sort"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"
)

func dumpFingerprint(d *dto.Fingerprint, f *clientmodel.Fingerprint) {
	d.Reset()

	d.Signature = proto.String(f.String())
}

func loadFingerprint(f *clientmodel.Fingerprint, d *dto.Fingerprint) {
	f.LoadFromString(d.GetSignature())
}

func dumpMetric(d *dto.Metric, m clientmodel.Metric) {
	d.Reset()

	metricLength := len(m)
	labelNames := make([]string, 0, metricLength)

	for labelName := range m {
		labelNames = append(labelNames, string(labelName))
	}

	sort.Strings(labelNames)

	pairs := make([]*dto.LabelPair, 0, metricLength)

	for _, labelName := range labelNames {
		l := clientmodel.LabelName(labelName)
		labelValue := m[l]
		labelPair := &dto.LabelPair{
			Name:  proto.String(string(labelName)),
			Value: proto.String(string(labelValue)),
		}

		pairs = append(pairs, labelPair)
	}

	d.LabelPair = pairs
}

func dumpLabelName(d *dto.LabelName, l clientmodel.LabelName) {
	d.Reset()

	d.Name = proto.String(string(l))
}
