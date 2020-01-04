package promql

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
)

func BenchmarkSignature(b *testing.B) {
	for n := 0; n < b.N; n++ {
		resultMetric(labels.Labels{labels.Label{Name: "y", Value: "y"}}, labels.Labels{labels.Label{Name: "x", Value: "x"}}, ADD, &VectorMatching{Card: CardOneToOne}, &EvalNodeHelper{out: make(Vector, 0, 1)})
	}
}

func BenchmarkSignatureReuse(b *testing.B) {
	ehn := &EvalNodeHelper{out: make(Vector, 0, 1)}
	for n := 0; n < b.N; n++ {
		resultMetric(labels.Labels{labels.Label{Name: "y", Value: "y"}}, labels.Labels{labels.Label{Name: "x", Value: "x"}}, ADD, &VectorMatching{Card: CardOneToOne}, ehn)
	}
}
