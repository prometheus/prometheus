package exemplar

import "github.com/prometheus/prometheus/pkg/labels"

// Exemplar is additional information associated with a metric.
type Exemplar struct {
	Labels labels.Labels
	Value  float64
	Ts     *int64
}
