module github.com/prometheus/prometheus/promql/v2

go 1.14

require (
	github.com/edsrzf/mmap-go v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.1
	github.com/prometheus/common v0.9.1
	github.com/prometheus/prometheus v2.16.0+incompatible
	github.com/prometheus/prometheus/promql v0.0.0-00010101000000-000000000000
	github.com/prometheus/prometheus/tsdb v0.0.0-00010101000000-000000000000
)

replace (
	github.com/prometheus/prometheus => ../
	github.com/prometheus/prometheus/promql => ./
	github.com/prometheus/prometheus/tsdb => ../tsdb
)
