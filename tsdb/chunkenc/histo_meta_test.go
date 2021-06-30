package chunkenc

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/histogram"
	"github.com/stretchr/testify/require"
)

// example of a span layout and resulting bucket indices (_idx_ is used in this histogram, others are shown just for context)
// spans      : [offset: 0, length: 2]                [offset 1, length 1]
// bucket idx : _0_                _1_      2         [3]                  4 ...

func TestBucketIterator(t *testing.T) {
	type test struct {
		spans []histogram.Span
		idxs  []int
	}
	tests := []test{
		{
			spans: []histogram.Span{
				{
					Offset: 0,
					Length: 1,
				},
			},
			idxs: []int{0},
		},
		{
			spans: []histogram.Span{
				{
					Offset: 0,
					Length: 2,
				},
				{
					Offset: 1,
					Length: 1,
				},
			},
			idxs: []int{0, 1, 3},
		},
		{
			spans: []histogram.Span{
				{
					Offset: 100,
					Length: 4,
				},
				{
					Offset: 8,
					Length: 7,
				},
				{
					Offset: 0,
					Length: 1,
				},
			},
			idxs: []int{100, 101, 102, 103, 112, 113, 114, 115, 116, 117, 118, 119},
		},
	}
	for _, test := range tests {
		b := newBucketIterator(test.spans)
		var got []int
		v, ok := b.Next()
		for ok {
			got = append(got, v)
			v, ok = b.Next()
		}
		require.Equal(t, test.idxs, got)
	}
}
