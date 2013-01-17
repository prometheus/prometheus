package api

import (
	"code.google.com/p/gorest"
)

type MetricsService struct {
	gorest.RestService `root:"/api/" consumes:"application/json" produces:"application/json"`

	query      gorest.EndPoint `method:"GET" path:"/query?{expr:string}&{json:string}" output:"string"`
	queryRange gorest.EndPoint `method:"GET" path:"/query_range?{expr:string}&{end:int64}&{range:int64}&{step:int64}" output:"string"`
}
