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

//go:build linux

package rules

import (
	"context"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

// BenchmarkRuleGroupSeriesResolution measures the recording-rule write path
// with stable output series. It uses a fixed query result to exclude PromQL
// execution and a primary-only TSDB fanout with the WAL enabled.
func BenchmarkRuleGroupSeriesResolution(b *testing.B) {
	const outputs = 10000

	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	db, err := tsdb.Open(b.TempDir(), logger, reg, tsdb.DefaultOptions(), nil)
	if err != nil {
		b.Fatal(err)
	}
	db.DisableCompactions()
	fanout := storage.NewFanout(logger, db)
	b.Cleanup(func() {
		if err := fanout.Close(); err != nil {
			b.Error(err)
		}
	})

	base := make(promql.Vector, outputs)
	for i := range base {
		base[i] = promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "input_metric", "series_id", strconv.Itoa(i),
				"cluster", "cluster-a", "instance", "host-0", "job", "input",
				"namespace", "ns", "zone", "zone-a", "pod", "pod-0",
				"endpoint", "/metrics", "method", "GET",
			),
			F: float64(i),
		}
	}

	rule := NewRecordingRule("recorded_metric", &parser.NumberLiteral{Val: 1}, labels.EmptyLabels())
	group := NewGroup(GroupOptions{
		Name:     "series-resolution",
		File:     "benchmark.rules",
		Interval: 15 * time.Second,
		Rules:    []Rule{rule},
		Opts: &ManagerOptions{
			Logger:       logger,
			Registerer:   reg,
			AppendableV2: fanout,
			QueryFunc: func(_ context.Context, _ string, at time.Time) (promql.Vector, error) {
				vector := make(promql.Vector, len(base))
				copy(vector, base)
				for i := range vector {
					vector[i].T = at.UnixMilli()
				}
				return vector, nil
			},
		},
	})

	ctx := context.Background()
	at := time.Unix(1700000000, 0)
	DefaultEvalIterationFunc(ctx, group, at)
	if err := rule.LastError(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	startCPU := benchmarkRuleGroupCPUTime(b)
	for b.Loop() {
		at = at.Add(15 * time.Second)
		DefaultEvalIterationFunc(ctx, group, at)
		if err := rule.LastError(); err != nil {
			b.Fatal(err)
		}
	}
	cpuTime := benchmarkRuleGroupCPUTime(b) - startCPU
	b.ReportMetric(float64(cpuTime)/float64(b.N), "cpu-ns/op")
}

func benchmarkRuleGroupCPUTime(b *testing.B) int64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		b.Fatal(err)
	}
	return usage.Utime.Nano() + usage.Stime.Nano()
}
