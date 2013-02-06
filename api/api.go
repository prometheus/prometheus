package api

import (
	"code.google.com/p/gorest"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

type MetricsService struct {
	gorest.RestService `root:"/api/" consumes:"application/json" produces:"application/json"`

	query      gorest.EndPoint `method:"GET" path:"/query?{expr:string}&{json:string}" output:"string"`
	queryRange gorest.EndPoint `method:"GET" path:"/query_range?{expr:string}&{end:int64}&{range:int64}&{step:int64}" output:"string"`
	metrics    gorest.EndPoint `method:"GET" path:"/metrics" output:"string"`

	persistence metric.MetricPersistence
	time        utility.Time
}

func NewMetricsService(p metric.MetricPersistence) *MetricsService {
	return &MetricsService{
		persistence: p,
	}
}
