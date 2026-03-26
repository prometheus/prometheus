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

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFQName(t *testing.T) {
	tests := []struct {
		name string
		m    metric
		want string
	}{
		{
			name: "all parts",
			m:    metric{Namespace: "prometheus", Subsystem: "engine", RawName: "queries"},
			want: "prometheus_engine_queries",
		},
		{
			name: "namespace and name only",
			m:    metric{Namespace: "prometheus", RawName: "ready"},
			want: "prometheus_ready",
		},
		{
			name: "name only",
			m:    metric{RawName: "queries"},
			want: "queries",
		},
		{
			name: "empty",
			m:    metric{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.fqName(); got != tt.want {
				t.Errorf("fqName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCategoryFromPackage(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"tsdb/wlog", "TSDB"},
		{"tsdb", "TSDB"},
		{"scrape", "Scrape"},
		{"rules", "Rules"},
		{"promql", "PromQL"},
		{"notifier", "Notifier"},
		{"storage/remote", "Remote storage"},
		{"storage", "Storage"},
		{"discovery/kubernetes", "Service discovery"},
		{"web", "Web"},
		{"template", "Template"},
		{"cmd/prometheus", "Server"},
		{"util/notifications", "Utilities"},
		{"something/else", "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			if got := categoryFromPackage(tt.pkg); got != tt.want {
				t.Errorf("categoryFromPackage(%q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

func TestParseFileBasicMetrics(t *testing.T) {
	src := `package example

import "github.com/prometheus/client_golang/prometheus"

var myCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "prometheus",
	Subsystem: "test",
	Name:      "requests_total",
	Help:      "Total number of requests.",
})

var myGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: "prometheus",
	Name:      "ready",
	Help:      "Whether the server is ready.",
})

var myHist = prometheus.NewHistogram(prometheus.HistogramOpts{
	Namespace: "prometheus",
	Subsystem: "http",
	Name:      "duration_seconds",
	Help:      "Request duration.",
})
`
	metrics := parseTestSource(t, src)

	want := []struct {
		fqName string
		typ    string
		help   string
	}{
		{"prometheus_test_requests_total", "counter", "Total number of requests."},
		{"prometheus_ready", "gauge", "Whether the server is ready."},
		{"prometheus_http_duration_seconds", "histogram", "Request duration."},
	}

	if len(metrics) != len(want) {
		t.Fatalf("got %d metrics, want %d", len(metrics), len(want))
	}

	for i, w := range want {
		if metrics[i].fqName() != w.fqName {
			t.Errorf("metric %d: fqName() = %q, want %q", i, metrics[i].fqName(), w.fqName)
		}
		if metrics[i].Type != w.typ {
			t.Errorf("metric %d: Type = %q, want %q", i, metrics[i].Type, w.typ)
		}
		if metrics[i].Help != w.help {
			t.Errorf("metric %d: Help = %q, want %q", i, metrics[i].Help, w.help)
		}
	}
}

func TestParseFileConstantResolution(t *testing.T) {
	src := `package example

import "github.com/prometheus/client_golang/prometheus"

const (
	namespace = "prometheus"
	subsystem = "notifications"
)

var dropped = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystem,
	Name:      "dropped_total",
	Help:      "Total number of dropped notifications.",
})
`
	metrics := parseTestSource(t, src)

	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if got := metrics[0].fqName(); got != "prometheus_notifications_dropped_total" {
		t.Errorf("fqName() = %q, want %q", got, "prometheus_notifications_dropped_total")
	}
}

func TestParseFileConcatenatedConstants(t *testing.T) {
	src := `package example

import "github.com/prometheus/client_golang/prometheus"

const (
	BaseNamespace     = "prometheus_sd_kubernetes"
	WorkqueueNS       = BaseNamespace + "_workqueue"
)

var depth = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: WorkqueueNS,
	Name:      "depth",
	Help:      "Current depth of the work queue.",
})
`
	metrics := parseTestSource(t, src)

	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if got := metrics[0].fqName(); got != "prometheus_sd_kubernetes_workqueue_depth" {
		t.Errorf("fqName() = %q, want %q", got, "prometheus_sd_kubernetes_workqueue_depth")
	}
}

func TestParseFileStringConcatenationInHelp(t *testing.T) {
	src := `package example

import "github.com/prometheus/client_golang/prometheus"

var m = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "bytes_total",
	Help: "Total bytes" + " processed.",
})
`
	metrics := parseTestSource(t, src)

	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if got := metrics[0].Help; got != "Total bytes processed." {
		t.Errorf("Help = %q, want %q", got, "Total bytes processed.")
	}
}

func TestParseFileSkipsMetricsWithoutName(t *testing.T) {
	src := `package example

import "github.com/prometheus/client_golang/prometheus"

var m = prometheus.NewCounter(prometheus.CounterOpts{
	Help: "A metric without a name.",
})
`
	metrics := parseTestSource(t, src)

	if len(metrics) != 0 {
		t.Fatalf("got %d metrics, want 0 (no Name field)", len(metrics))
	}
}

func TestParseFileCrossPackageConstant(t *testing.T) {
	src := `package example

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery"
)

var m = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: discovery.KubernetesMetricsNamespace,
	Name:      "events_total",
	Help:      "The number of events handled.",
})
`
	allConsts := map[string]map[string]string{
		"discovery": {
			"KubernetesMetricsNamespace": "prometheus_sd_kubernetes",
		},
	}

	metrics := parseTestSourceWithConsts(t, src, nil, allConsts)

	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if got := metrics[0].fqName(); got != "prometheus_sd_kubernetes_events_total" {
		t.Errorf("fqName() = %q, want %q", got, "prometheus_sd_kubernetes_events_total")
	}
}

func TestCollectPkgConstants(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "consts.go"), `package example

const (
	namespace = "prometheus"
	subsystem = "test"
	combined  = namespace + "_extra"
)
`)

	consts, err := collectPkgConstants(dir)
	if err != nil {
		t.Fatal(err)
	}

	pkgConsts := consts["."]
	if pkgConsts == nil {
		t.Fatal("no constants found for package '.'")
	}

	tests := map[string]string{
		"namespace": "prometheus",
		"subsystem": "test",
		"combined":  "prometheus_extra",
	}
	for name, want := range tests {
		if got := pkgConsts[name]; got != want {
			t.Errorf("const %q = %q, want %q", name, got, want)
		}
	}
}

// parseTestSource writes source to a temp file and parses it with no
// pre-existing constants.
func parseTestSource(t *testing.T, src string) []metric {
	t.Helper()
	return parseTestSourceWithConsts(t, src, nil, nil)
}

// parseTestSourceWithConsts writes source to a temp file and parses it
// with the given constants maps.
func parseTestSourceWithConsts(t *testing.T, src string, consts map[string]string, allConsts map[string]map[string]string) []metric {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	writeFile(t, path, src)

	// If consts is nil, try to extract from the source itself.
	if consts == nil {
		pkgConsts, err := collectPkgConstants(dir)
		if err != nil {
			t.Fatal(err)
		}
		consts = pkgConsts["."]
		if allConsts == nil {
			allConsts = pkgConsts
		}
	}

	metrics, err := parseFile(path, "test.go", ".", consts, allConsts)
	if err != nil {
		t.Fatal(err)
	}
	return metrics
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
