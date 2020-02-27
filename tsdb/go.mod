module github.com/prometheus/prometheus/tsdb/v2

go 1.14

require (
	github.com/cespare/xxhash v1.1.0
	github.com/dgryski/go-sip13 v0.0.0-20190329191031-25c5027a8c7b
	github.com/go-kit/kit v0.10.0
	github.com/golang/snappy v0.0.1
	github.com/oklog/ulid v1.3.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.1
	github.com/prometheus/prometheus v2.16.0+incompatible
	github.com/prometheus/prometheus/tsdb v0.0.0-00010101000000-000000000000
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200223170610-d5e6a3e2c0ae
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

replace (
	github.com/prometheus/prometheus => ../
	github.com/prometheus/prometheus/promql => ../promql
	github.com/prometheus/prometheus/tsdb => ./
)
