package api

import (
	"code.google.com/p/gorest"
)

type MetricsService struct {
	gorest.RestService `root:"/api/" consumes:"application/json" produces:"application/json"`

	query gorest.EndPoint `method:"GET" path:"/query?{expr:string}&{json:string}&{start:string}&{end:string}" output:"string"`
}
