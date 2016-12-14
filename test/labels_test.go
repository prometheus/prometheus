package test

import (
	"testing"

	"github.com/fabxc/tsdb"
)

func BenchmarkLabelMapAccess(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}

	var v string

	for k := range m {
		b.Run(k, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				v = m[k]
			}
		})
	}

	_ = v
}

func BenchmarkLabelSetAccess(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := tsdb.LabelsFromMap(m)

	var v string

	for _, l := range ls {
		b.Run(l.Name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				v = ls.Get(l.Name)
			}
		})
	}

	_ = v
}
