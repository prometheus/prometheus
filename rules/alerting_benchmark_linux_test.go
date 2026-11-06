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

package rules

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"golang.org/x/sys/unix"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

const alertingTemplateBenchmarkSeries = 1000

func seedAlertingTemplateBenchmark(b *testing.B) *teststorage.TestStorage {
	b.Helper()
	st := teststorage.New(b)
	app := st.AppenderV2(context.Background())
	for i := range alertingTemplateBenchmarkSeries {
		ls := labels.FromStrings(
			labels.MetricName, "up",
			"instance", fmt.Sprintf("node-%05d:9090", i),
			"job", "probe",
			"cluster", "east",
			"namespace", "monitoring",
			"service", "node",
		)
		if _, err := app.Append(0, ls, 0, 0, 1, nil, nil, storage.AOptions{}); err != nil {
			b.Fatal(err)
		}
	}
	if err := app.Commit(); err != nil {
		b.Fatal(err)
	}
	return st
}

func alertingTemplateBenchmarkCPU(b *testing.B) time.Duration {
	b.Helper()
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		b.Fatal(err)
	}
	return time.Duration(usage.Utime.Sec+usage.Stime.Sec)*time.Second + time.Duration(usage.Utime.Usec+usage.Stime.Usec)*time.Microsecond
}

func BenchmarkAlertingRuleEval(b *testing.B) {
	st := seedAlertingTemplateBenchmark(b)
	engine := promqltest.NewTestEngine(b, false, 24*time.Hour, 1_000_000)
	expr, err := testParser.ParseExpr("up > 0")
	if err != nil {
		b.Fatal(err)
	}
	logger := promslog.NewNopLogger()
	externalURL, err := url.Parse("http://localhost:9090")
	if err != nil {
		b.Fatal(err)
	}
	rule := NewAlertingRule(
		"NodeAvailable",
		expr,
		0,
		0,
		labels.FromStrings("severity", "warning", "owner", "{{ $labels.namespace }}"),
		labels.FromStrings(
			"summary", "Node {{ $labels.instance }} is available",
			"description", "{{ $labels.job }} in {{ $externalLabels.cluster }} has value {{ $value }}",
		),
		labels.FromStrings("cluster", "east"),
		externalURL.String(),
		true,
		logger,
	)
	group := NewGroup(GroupOptions{
		Name:     "alert-template-benchmark",
		File:     "alert-template-benchmark.rules",
		Interval: 15 * time.Second,
		Rules:    []Rule{rule},
		Opts: &ManagerOptions{
			QueryFunc:    EngineQueryFunc(engine, st),
			AppendableV2: st,
			Queryable:    st,
			Context:      context.Background(),
			Logger:       logger,
			ExternalURL:  externalURL,
			ResendDelay:  time.Minute,
			NotifyFunc:   func(context.Context, string, ...*Alert) {},
		},
	})

	start := time.Unix(1, 0)
	DefaultEvalIterationFunc(context.Background(), group, start)
	if got, want := len(group.seriesInPreviousEval[0]), 2*alertingTemplateBenchmarkSeries; got != want {
		b.Fatalf("tracked output series: got %d, want %d", got, want)
	}

	b.ReportAllocs()
	b.ResetTimer()
	cpuStart := alertingTemplateBenchmarkCPU(b)
	for i := 0; i < b.N; i++ {
		ts := start.Add(time.Duration(i+1) * time.Millisecond)
		DefaultEvalIterationFunc(context.Background(), group, ts)
	}
	cpuEnd := alertingTemplateBenchmarkCPU(b)
	b.StopTimer()
	if b.N == 0 {
		b.Fatal("benchmark ran no evaluations")
	}
	b.ReportMetric(float64(cpuEnd-cpuStart)/float64(b.N), "cpu-ns/op")
	if got, want := len(group.seriesInPreviousEval[0]), 2*alertingTemplateBenchmarkSeries; got != want {
		b.Fatalf("tracked output series: got %d, want %d", got, want)
	}
}
