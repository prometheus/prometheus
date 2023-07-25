// Copyright 2021 The Prometheus Authors
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

package promql

import (
	"context"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/tsdb"
	"go.opentelemetry.io/otel"
	"os"
	"runtime"
	"testing"
	"time"
)

func BenchmarkCountSingleInstance(b *testing.B) {
	benchmarkQuery("count(bid_request_count{instance='vad-gp-bac12-1:11108'})", b)
}

func BenchmarkCountAll(b *testing.B) {
	benchmarkQuery("count(bid_request_count)", b)
}

func BenchmarkCountSingleInstanceOver1h(b *testing.B) {
	benchmarkQuery("count(count_over_time(bid_request_count{instance='vad-gp-bac12-1:11108'}[1h]))", b)
}

func BenchmarkCountAllOver1h(b *testing.B) {
	benchmarkQuery("count(count_over_time(bid_request_count[1h]))", b)
}

func benchmarkQuery(query string, b *testing.B) {
	// Open a TSDB for reading and/or writing.
	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	l = level.NewFilter(l, level.AllowError())
	// TSDB contains a single production block, containing a high cardinality series (~150K)
	db, err := tsdb.Open("/temp/test-tsdb", l, nil, tsdb.DefaultOptions(), nil)
	noErr(err)

	// Create engine for querying
	otel.Tracer("name").Start(context.Background(), "Run")

	for n := 0; n < b.N; n++ {
		eng := NewEngine(EngineOpts{
			Logger:             l,
			Reg:                nil,
			MaxSamples:         50000000,
			Timeout:            60 * time.Second,
			EnablePerStepStats: true,
		})
		noErr(err)
		q, err := eng.NewInstantQuery(context.Background(), db, &PrometheusQueryOpts{}, query, time.UnixMilli(1688922000001))
		noErr(err)
		q.Stats().Samples.EnablePerStepStats = true
		b.StartTimer()
		q.Exec(context.Background())
		b.StopTimer()

		if n == 0 {
			b.ReportMetric(float64(q.Stats().Samples.PeakSamples), "peak_samples")
			b.ReportMetric(float64(q.Stats().Samples.TotalSamples), "total_samples")
		}
	}

	var memstats runtime.MemStats
	runtime.ReadMemStats(&memstats)
	memstats.Sys
	b.ReportMetric(float64(memstats.Sys), "sys_memory")

	// Clean up any last resources when done.
	err = db.Close()
	noErr(err)
}

func noErr(err error) {
	if err != nil {
		panic(err)
	}
}
