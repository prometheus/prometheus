package histogram

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMergeFloatBucketIterator(t *testing.T) {
	cases := []struct {
		histograms      []Histogram
		expectedBuckets []FloatBucket
	}{
		{
			histograms: []Histogram{
				{
					Schema: 0,
					PositiveSpans: []Span{
						{Offset: 0, Length: 2},
						{Offset: 1, Length: 2},
					},
					PositiveBuckets: []int64{1, 1, -1, 0},
				},
				{
					Schema: 0,
					PositiveSpans: []Span{
						{Offset: 0, Length: 4},
						{Offset: 0, Length: 0},
						{Offset: 0, Length: 3},
					},
					PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				},
			},
			expectedBuckets: []FloatBucket{
				{Lower: 0.5, Upper: 1, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 5, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 2, Upper: 4, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 2},
				{Lower: 4, Upper: 8, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 16, Upper: 32, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 5},
				{Lower: 32, Upper: 64, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 6},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			its := make([]FloatBucketIterator, 0, len(c.histograms))
			for _, h := range c.histograms {
				its = append(its, h.ToFloat().PositiveBucketIterator())
			}

			it := NewMergeFloatBucketIterator(its...)
			actualBuckets := make([]FloatBucket, 0, len(c.expectedBuckets))
			for it.Next() {
				actualBuckets = append(actualBuckets, it.At())
			}
			require.Equal(t, c.expectedBuckets, actualBuckets)
		})
	}
}
