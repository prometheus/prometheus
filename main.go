package main

import (
	"code.google.com/p/gorest"
	"github.com/matttproud/prometheus/storage/metric/leveldb"
	"net/http"
)

func main() {
	m, _ := leveldb.NewLevigoMetricPersistence("/tmp/metrics")
	s := &MetricsService{
		persistence: m,
	}
	gorest.RegisterService(s)
	http.Handle("/", gorest.Handle())
	http.ListenAndServe(":8787", nil)
}
