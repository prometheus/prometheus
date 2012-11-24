package main

import (
	"code.google.com/p/gorest"
)

type MetricsService struct {
	gorest.RestService `root:"/" consumes:"application/json" produces:"application/json"`

	persistence *LevigoMetricPersistence

	listLabels     gorest.EndPoint `method:"GET" path:"/labels/" output:"[]string"`
	listLabelPairs gorest.EndPoint `method:"GET" path:"/label-pairs/" output:"[]LabelPairs"`
	listMetrics    gorest.EndPoint `method:"GET" path:"/metrics/" output:"[]LabelPairs"`

	appendSample gorest.EndPoint `method:"POST" path:"/metrics/" postdata:"Sample"`
}

func (m MetricsService) ListLabels() []string {
	labels, labelsError := m.persistence.GetLabelNames()

	if labelsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return labels
}

func (m MetricsService) ListLabelPairs() []LabelPairs {
	labelPairs, labelPairsError := m.persistence.GetLabelPairs()

	if labelPairsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return labelPairs
}

func (m MetricsService) ListMetrics() []LabelPairs {
	metrics, metricsError := m.persistence.GetMetrics()

	if metricsError != nil {
		m.ResponseBuilder().SetResponseCode(500)
	}

	return metrics
}

func (m MetricsService) AppendSample(s Sample) {
	responseBuilder := m.ResponseBuilder()
	if appendError := m.persistence.AppendSample(&s); appendError == nil {
		responseBuilder.SetResponseCode(200)
		return
	}

	responseBuilder.SetResponseCode(500)
}
