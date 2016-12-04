package tsdb

import "testing"

func BenchmarkLabelSetFromMap(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	var ls Labels
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ls = LabelsFromMap(m)
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
	ls := LabelsFromMap(m)

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
	ls := LabelsFromMap(m)
	var res bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res = ls.Equals(ls)
	}
	_ = res
}
