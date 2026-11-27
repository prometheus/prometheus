// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0

package storage

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestBufferedSeriesIteratorSeekPreloadedSample(t *testing.T) {
	for _, tc := range []struct {
		name          string
		delta, target int64
	}{
		{"small lookback", 1, math.MinInt64 + 1},
		{"large lookback", math.MaxInt64, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reset := range []bool{false, true} {
				t.Run(map[bool]string{false: "constructor", true: "reset"}[reset], func(t *testing.T) {
					source := NewListSeriesIterator(samples{fSample{t: 1, f: 11}, fSample{t: 2, f: 22}})
					var b *BufferedSeriesIterator
					if reset {
						b = NewBuffer(tc.delta)
						b.Reset(source)
					} else {
						b = NewBufferIterator(source, tc.delta)
					}
					require.Equal(t, chunkenc.ValFloat, b.Seek(tc.target))
					ts, v := b.At()
					require.Equal(t, int64(1), ts)
					require.Equal(t, 11.0, v)
					_, ok := b.PeekBack(1)
					require.False(t, ok)
					require.Equal(t, chunkenc.ValFloat, b.Next())
					require.Equal(t, int64(2), b.AtT())
				})
			}
		})
	}
}
