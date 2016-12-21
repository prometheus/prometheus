package tsdb

import (
	"testing"

	"github.com/fabxc/tsdb/labels"
)

func BenchmarkLabelSetFromMap(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	var ls labels.Labels
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ls = labels.FromMap(m)
	}
	_ = ls
}

func BenchmarkMapFromLabels(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := labels.FromMap(m)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m = ls.Map()
	}
}

func BenchmarkLabelSetEquals(b *testing.B) {
	// The vast majority of comparisons will be against a matching label set.
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := labels.FromMap(m)
	var res bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res = ls.Equals(ls)
	}
	_ = res
}
