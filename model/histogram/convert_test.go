package histogram

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNHCBtoClassic(t *testing.T) {
	tests := []struct {
		name        string
		input       *FloatHistogram
		output      *TempHistogram
		isErrNeeded bool
	}{
		{
			name:        "nil input",
			input:       nil,
			output:      nil,
			isErrNeeded: true,
		},
		{
			name: "exponential schema",
			input: &FloatHistogram{
				Schema: -52,
			},
			output:      nil,
			isErrNeeded: true,
		},
		{
			name: "no custom values",
			input: &FloatHistogram{
				Schema:       -53,
				CustomValues: []float64{},
			},
			output:      nil,
			isErrNeeded: true,
		},
		{
			name: "mismatched buckets and custom values",
			input: &FloatHistogram{
				Schema:          -53,
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{5, 10},
			},
			output:      nil,
			isErrNeeded: true,
		},
		{
			name: "valid NHCB histogram",
			input: &FloatHistogram{
				Schema:          -53,
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{5, 10, 15},
				Count:           30,
				Sum:             60,
			},
			output: &TempHistogram{
				buckets: []tempHistogramBucket{
					{le: 1, count: 5},
					{le: 2, count: 15},
					{le: 3, count: 30},
					{le: float64(math.Inf(1)), count: 30},
				},
				count:    30,
				sum:      60,
				err:      nil,
				hasCount: true,
			},
			isErrNeeded: false,
		},
		{
			name: "valid NHCB histogram with zero counts",
			input: &FloatHistogram{
				Schema:          -53,
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{0, 0, 0},
				Count:           0,
				Sum:             0,
			},
			output: &TempHistogram{
				buckets: []tempHistogramBucket{
					{le: 1, count: 0},
					{le: 2, count: 0},
					{le: 3, count: 0},
					{le: float64(math.Inf(1)), count: 0},
				},
				count:    0,
				sum:      0,
				err:      nil,
				hasCount: true,
			},
			isErrNeeded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertNHCBToClassicHistogram(tt.input)

			if tt.isErrNeeded {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				require.Equal(t, tt.output.count, result.count, "count mismatch")
				require.Equal(t, tt.output.sum, result.sum, "sum mismatch")
				require.Equal(t, tt.output.hasCount, result.hasCount, "hasCount mismatch")

				require.Len(t, result.buckets, len(tt.output.buckets), "bucket count mismatch")

				for i := range result.buckets {
					require.Equal(t, tt.output.buckets[i].le, result.buckets[i].le, "bucket[%d].le mismatch", i)
					require.Equal(t, tt.output.buckets[i].count, result.buckets[i].count, "bucket[%d].count mismatch", i)
				}
			}
		})
	}
}
