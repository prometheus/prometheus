// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0

package storage

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestBufferedSeriesIteratorSeekLookbackUnderflow(t *testing.T) {
	for _, delta := range []int64{1, 2, math.MaxInt64} {
		t.Run(strconv.FormatInt(delta, 10), func(t *testing.T) {
			b := NewBufferIterator(NewListSeriesIterator(samples{fSample{t: 1, f: 11}, fSample{t: 2, f: 22}, fSample{t: 3, f: 33}}), delta)
			require.Equal(t, chunkenc.ValFloat, b.Next())
			require.Equal(t, int64(2), b.AtT())
			require.Equal(t, chunkenc.ValFloat, b.Seek(math.MinInt64))
			ts, v := b.At()
			require.Equal(t, int64(2), ts)
			require.Equal(t, 22.0, v)
			sample, ok := b.PeekBack(1)
			require.True(t, ok)
			require.Equal(t, int64(1), sample.T())
			require.Equal(t, 11.0, sample.F())
			require.Equal(t, chunkenc.ValFloat, b.Seek(math.MinInt64))
			require.Equal(t, chunkenc.ValFloat, b.Next())
			require.Equal(t, int64(3), b.AtT())
		})
	}
}
