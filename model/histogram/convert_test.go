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
				Buckets: []TempHistogramBucket{
					{Le: 1, Count: 5},
					{Le: 2, Count: 15},
					{Le: 3, Count: 30},
					{Le: float64(math.Inf(1)), Count: 30},
				},
				Count:    30,
				Sum:      60,
				Err:      nil,
				HasCount: true,
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
				Buckets: []TempHistogramBucket{
					{Le: 1, Count: 0},
					{Le: 2, Count: 0},
					{Le: 3, Count: 0},
					{Le: float64(math.Inf(1)), Count: 0},
				},
				Count:    0,
				Sum:      0,
				Err:      nil,
				HasCount: true,
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

				require.Equal(t, tt.output.Count, result.Count, "count mismatch")
				require.Equal(t, tt.output.Sum, result.Sum, "sum mismatch")
				require.Equal(t, tt.output.HasCount, result.HasCount, "hasCount mismatch")

				require.Len(t, result.Buckets, len(tt.output.Buckets), "bucket count mismatch")

				for i := range result.Buckets {
					require.Equal(t, tt.output.Buckets[i].Le, result.Buckets[i].Le, "bucket[%d].le mismatch", i)
					require.Equal(t, tt.output.Buckets[i].Count, result.Buckets[i].Count, "bucket[%d].count mismatch", i)
				}
			}
		})
	}
}
