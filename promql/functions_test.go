// Copyright 2015 The Prometheus Authors
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
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/util/testutil"
)

func BenchmarkHoltWinters4Week5Min(b *testing.B) {
	input := `
clear
load 5m
    http_requests{path="/foo"}    0+10x8064

eval instant at 4w holt_winters(http_requests[4w], 0.3, 0.3)
    {path="/foo"} 80640
`

	bench := NewBenchmark(b, input)
	bench.Run()

}

func BenchmarkHoltWinters1Week5Min(b *testing.B) {
	input := `
clear
load 5m
    http_requests{path="/foo"}    0+10x2016

eval instant at 1w holt_winters(http_requests[1w], 0.3, 0.3)
    {path="/foo"} 20160
`

	bench := NewBenchmark(b, input)
	bench.Run()
}

func BenchmarkHoltWinters1Day1Min(b *testing.B) {
	input := `
clear
load 1m
    http_requests{path="/foo"}    0+10x1440

eval instant at 1d holt_winters(http_requests[1d], 0.3, 0.3)
    {path="/foo"} 14400
`

	bench := NewBenchmark(b, input)
	bench.Run()
}

func BenchmarkChanges1Day1Min(b *testing.B) {
	input := `
clear
load 1m
    http_requests{path="/foo"}    0+10x1440

eval instant at 1d changes(http_requests[1d])
    {path="/foo"} 1440
`

	bench := NewBenchmark(b, input)
	bench.Run()
}

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	storage := testutil.NewStorage(t)
	defer storage.Close()
	engine := NewEngine(storage, nil)

	a, err := storage.Appender()
	if err != nil {
		t.Fatal(err)
	}

	metric := labels.FromStrings("__name__", "foo")
	a.Add(metric, 1493712816939, 1.0)
	a.Add(metric, 1493712846939, 1.0)

	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}

	query, err := engine.NewInstantQuery("deriv(foo[30m])", timestamp.Time(1493712846939))
	if err != nil {
		t.Fatalf("Error parsing query: %s", err)
	}
	result := query.Exec(context.Background())
	if result.Err != nil {
		t.Fatalf("Error running query: %s", result.Err)
	}
	vec, _ := result.Vector()
	if len(vec) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(vec))
	}
	if vec[0].V != 0.0 {
		t.Fatalf("Expected 0.0 as value, got %f", vec[0].V)
	}
}
