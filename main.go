package main

import (
	"code.google.com/p/gorest"
	"net/http"
)

func main() {
	m, _ := NewLevigoMetricPersistence("/tmp/metrics")
	s := &MetricsService{
		persistence: m,
	}
	gorest.RegisterService(s)
	http.Handle("/", gorest.Handle())
	http.ListenAndServe(":8787", nil)
}
