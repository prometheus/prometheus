package almost

import (
	"math"
	"testing"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		a        float64
		b        float64
		epsilon  float64
		expected bool
	}{
		{1.0, 1.0, 0.0001, true},
		{1.0, 1.0001, 0.0001, true},
		{1.0, 1.001, 0.0001, false},
		{0, 0, 0.0001, true},
		{1.0, 0, 0.0001, false},
		{-1.0, -1.0001, 0.0001, true},
		{math.NaN(), math.NaN(), 0.0001, true},
		{math.Inf(-1), math.Inf(-1), 0.0001, true},
	}

	for _, test := range tests {
		result := Equal(test.a, test.b, test.epsilon)
		if result != test.expected {
			t.Errorf("Equal(%v, %v, %v) = %t, expected %t", test.a, test.b, test.epsilon, result, test.expected)
		}
	}
}