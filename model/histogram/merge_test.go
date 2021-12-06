package histogram

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
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
						{Offset: 1, Length: 4},
						{Offset: 2, Length: 0},
						{Offset: 2, Length: 3},
					},
					PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				},
			},
			expectedBuckets: []FloatBucket{
				{Lower: 0.5, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 2, Upper: 4, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 2},
				{Lower: 4, Upper: 8, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 256, Upper: 512, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 9},
				{Lower: 512, Upper: 1024, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 10},
				{Lower: 1024, Upper: 2048, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 11},
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

func TestMergeHistograms(t *testing.T) {
	cases := []struct {
		histograms []FloatHistogram
		expected   FloatHistogram
	}{
		{
			histograms: []FloatHistogram{
				{
					Schema:        0,
					Count:         9,
					Sum:           1234.5,
					ZeroThreshold: 0.001,
					ZeroCount:     4,
					PositiveSpans: []Span{
						{Offset: 0, Length: 2},
						{Offset: 1, Length: 2},
					},
					PositiveBuckets: []float64{1, 2, 1, 1},
					NegativeSpans: []Span{
						{Offset: 0, Length: 2},
						{Offset: 2, Length: 2},
					},
					NegativeBuckets: []float64{2, 4, 1, 9},
				},
				{
					Schema:        0,
					Count:         15,
					Sum:           2345.6,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpans: []Span{
						{Offset: 0, Length: 4},
						{Offset: 0, Length: 0},
						{Offset: 0, Length: 3},
					},
					PositiveBuckets: []float64{1, 3, 1, 2, 1, 1, 1},
					NegativeSpans: []Span{
						{Offset: 1, Length: 4},
						{Offset: 2, Length: 0},
						{Offset: 2, Length: 3},
					},
					NegativeBuckets: []float64{1, 4, 2, 7, 5, 5, 2},
				},
			},
			expected: FloatHistogram{
				Schema:        0,
				ZeroThreshold: 0.001,
				ZeroCount:     9,
				Count:         24,
				Sum:           3580.1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 7},
				},
				PositiveBuckets: []float64{2, 5, 1, 3, 2, 1, 1},
				NegativeSpans: []Span{
					{Offset: 0, Length: 6},
					{Offset: 3, Length: 3},
				},
				NegativeBuckets: []float64{2, 5, 4, 2, 8, 9, 5, 5, 2},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			res := MergeHistograms(c.histograms...)
			require.Equal(t, c.expected, res)
		})
	}
}

func BenchmarkHistogramAdd(b *testing.B) {
	for _, numHists := range []int{10, 100, 1000, 10000} {
		for _, maxBuckets := range []int{10, 30, 50, 100, 150} {
			b.Run(fmt.Sprintf("numHists=%d,maxBuckets=%d", numHists, maxBuckets), func(b *testing.B) {
				hists := make([]FloatHistogram, 0, numHists)
				for x := 0; x < numHists; x++ {
					h := FloatHistogram{
						Schema:        0,
						Count:         4,
						Sum:           1234.5,
						ZeroThreshold: 0.001,
						ZeroCount:     4,
					}

					currBuckets := 0
					for currBuckets < maxBuckets {
						moreBuckets := rand.Intn(maxBuckets/3) + 1
						offset := rand.Intn(15) + 1
						h.PositiveSpans = append(h.PositiveSpans, Span{
							Offset: int32(offset),
							Length: uint32(moreBuckets),
						})
						for i := 0; i < moreBuckets; i++ {
							b := float64(rand.Intn(50))
							h.PositiveBuckets = append(h.PositiveBuckets, b)
							h.Count += b
						}
						currBuckets += moreBuckets
					}
					hists = append(hists, h)
				}

				b.ResetTimer()
				b.ReportAllocs()

				// Uncomment this for old way.
				//res := &hists[0]
				//for _, h := range hists[1:] {
				//	res = res.Add(&h)
				//}
				//_ = res

				// Uncomment this for new way.
				//_ = MergeHistograms(hists...)
			})
		}
	}

}
