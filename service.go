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

package main

import (
	"code.google.com/p/gorest"
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/storage/metric/leveldb"
)

type MetricsService struct {
	gorest.RestService `root:"/" consumes:"application/json" produces:"application/json"`

	persistence *leveldb.LevelDBMetricPersistence

	listLabels     gorest.EndPoint `method:"GET" path:"/labels/" output:"[]string"`
	listLabelPairs gorest.EndPoint `method:"GET" path:"/label-pairs/" output:"[]model.LabelPairs"`
	listMetrics    gorest.EndPoint `method:"GET" path:"/metrics/" output:"[]model.LabelPairs"`

	appendSample gorest.EndPoint `method:"POST" path:"/metrics/" postdata:"model.Sample"`
}

func (m MetricsService) ListLabels() []string {
	labels, labelsError := m.persistence.GetLabelNames()

	if labelsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return labels
}

func (m MetricsService) ListLabelPairs() []model.LabelPairs {
	labelPairs, labelPairsError := m.persistence.GetLabelPairs()

	if labelPairsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return labelPairs
}

func (m MetricsService) ListMetrics() []model.LabelPairs {
	metrics, metricsError := m.persistence.GetMetrics()

	if metricsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return metrics
}

func (m MetricsService) AppendSample(s model.Sample) {
	responseBuilder := m.ResponseBuilder()
	if appendError := m.persistence.AppendSample(&s); appendError == nil {
		responseBuilder.SetResponseCode(200)
		return
	}

	responseBuilder.SetResponseCode(500)
}
