package record

import (
	"testing"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/stretchr/testify/require"
)

func TestBuffersPool_PtrClear(t *testing.T) {
	pool := NewBuffersPool()
	
	h := pool.GetHistograms(1)
	h = append(h, RefHistogramSample{
		H: &histogram.Histogram{},
	})
	pool.PutHistograms(h)

	h2 := pool.GetHistograms(1)
	require.Len(t, h2, 0)
	require.Equal(t, 1, cap(h2))
	h2 = h2[:1] // extend to capacity to check previously stored item
	require.Nil(t, h2[0].H)
	
	fh := pool.GetFloatHistograms(1)
	fh = append(fh, RefFloatHistogramSample{
		FH: &histogram.FloatHistogram{},
	})
	pool.PutFloatHistograms(fh)

	fh2 := pool.GetFloatHistograms(1)
	require.Len(t, fh2, 0)
	require.Equal(t, 1, cap(fh2))
	fh2 = fh2[:1] // extend to capacity
	require.Nil(t, fh2[0].FH)
}
