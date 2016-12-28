package test

import "testing"

func BenchmarkMapConversion(b *testing.B) {
	type key string
	type val string

	m := map[key]val{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}

	var sm map[string]string

	for i := 0; i < b.N; i++ {
		sm = make(map[string]string, len(m))
		for k, v := range m {
			sm[string(k)] = string(v)
		}
	}
}

func BenchmarkListIter(b *testing.B) {
	var list []uint32
	for i := 0; i < 1e4; i++ {
		list = append(list, uint32(i))
	}

	b.ResetTimer()

	total := uint32(0)

	for j := 0; j < b.N; j++ {
		sum := uint32(0)
		for _, k := range list {
			sum += k
		}
		total += sum
	}
}
