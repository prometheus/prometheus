// Copyright The Prometheus Authors
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

package tsdb

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// BenchmarkDBAppenderV2Append measures concurrent DB AppenderV2 transaction writes.
// Run with -cpu=8 to benchmark eight concurrent appender workers.
func BenchmarkDBAppenderV2Append(b *testing.B) {
	benchmarkDBAppenderV2Append(b, runtime.GOMAXPROCS(0), 1)
}

// BenchmarkDBAppenderV2AppendSequential measures single-appender transaction writes for comparison.
func BenchmarkDBAppenderV2AppendSequential(b *testing.B) {
	benchmarkDBAppenderV2Append(b, 1, 1)
}

// BenchmarkDBAppenderV2AppendOversubscribed measures twice as many appender workers as Go processors.
func BenchmarkDBAppenderV2AppendOversubscribed(b *testing.B) {
	processors := runtime.GOMAXPROCS(0)
	benchmarkDBAppenderV2Append(b, processors*2, 2)
}

func benchmarkDBAppenderV2Append(b *testing.B, workers, parallelism int) {
	const outputsPerGroup = 10000
	const interval = 15 * time.Second

	logger := promslog.NewNopLogger()
	db, err := Open(b.TempDir(), logger, prometheus.NewRegistry(), DefaultOptions(), nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Error(err)
		}
	})

	ctx := context.Background()
	baseAt := time.Unix(1700000000, 0)
	seriesByWorker := make([][]labels.Labels, workers)
	for worker := range workers {
		series := make([]labels.Labels, outputsPerGroup)
		for i := range series {
			series[i] = labels.FromStrings(
				"__name__", "recorded_metric", "series_id", strconv.Itoa(i),
				"cluster", "cluster-a", "instance", fmt.Sprintf("host-%d", worker),
				"job", "input", "namespace", "ns", "zone", "zone-a",
				"pod", fmt.Sprintf("pod-%d", worker), "endpoint", "/metrics", "method", "GET",
			)
		}
		seriesByWorker[worker] = series
		if err := appendDBAppenderV2Batch(ctx, db, series, baseAt.UnixMilli()); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	if workers == 1 {
		at := baseAt
		for b.Loop() {
			at = at.Add(interval)
			if err := appendDBAppenderV2Batch(ctx, db, seriesByWorker[0], at.UnixMilli()); err != nil {
				b.Fatal(err)
			}
		}
	} else {
		nextWorker := atomic.NewUint64(0)
		b.SetParallelism(parallelism)
		b.RunParallel(func(pb *testing.PB) {
			worker := int(nextWorker.Inc() - 1)
			at := baseAt
			for pb.Next() {
				at = at.Add(interval)
				if err := appendDBAppenderV2Batch(ctx, db, seriesByWorker[worker], at.UnixMilli()); err != nil {
					b.Error(err)
					return
				}
			}
		})
	}
	b.StopTimer()
	elapsed := b.Elapsed()
	completedSamples := float64(b.N) * outputsPerGroup
	b.ReportMetric(completedSamples/elapsed.Seconds(), "samples/s")
}

func appendDBAppenderV2Batch(ctx context.Context, db *DB, series []labels.Labels, ts int64) error {
	app := db.AppenderV2(ctx)
	for i, lset := range series {
		if _, err := app.Append(0, lset, 0, ts, float64(i), nil, nil, storage.AOptions{}); err != nil {
			_ = app.Rollback()
			return err
		}
	}
	return app.Commit()
}
