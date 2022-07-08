package promql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestMatrix_ContainsSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		matrix   Matrix
		expected bool
	}{
		"empty matrix": {
			matrix:   Matrix{},
			expected: false,
		},
		"matrix with one series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: false,
		},
		"matrix with two different series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
			},
			expected: false,
		},
		"matrix with two equal series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
		"matrix with three series, two equal": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.matrix.ContainsSameLabelset())
		})
	}
}
