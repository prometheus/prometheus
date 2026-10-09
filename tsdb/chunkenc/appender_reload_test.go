// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0

package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestChunkAppenderAfterReload(t *testing.T) {
	for _, enc := range []Encoding{EncXOR, EncXOR2, EncHistogram, EncFloatHistogram, EncHistogramST, EncFloatHistogramST} {
		t.Run(enc.String(), func(t *testing.T) {
			c, err := NewEmptyChunk(enc)
			require.NoError(t, err)
			app, err := c.Appender()
			require.NoError(t, err)
			appendSample := func(ts int64) {
				switch enc {
				case EncHistogram, EncHistogramST:
					_, _, app, err = app.AppendHistogram(nil, 0, ts, &histogram.Histogram{}, false)
				case EncFloatHistogram, EncFloatHistogramST:
					_, _, app, err = app.AppendFloatHistogram(nil, 0, ts, &histogram.FloatHistogram{}, false)
				default:
					app.Append(0, ts, 1)
				}
				require.NoError(t, err)
			}
			appendSample(10)
			appendSample(11)
			c, err = FromData(enc, append([]byte(nil), c.Bytes()...))
			require.NoError(t, err)
			app, err = c.Appender()
			require.NoError(t, err)
			appendSample(13)
			it := c.Iterator(nil)
			for _, ts := range []int64{10, 11, 13} {
				require.NotEqual(t, ValNone, it.Next())
				require.Equal(t, ts, it.AtT())
			}
			require.Equal(t, ValNone, it.Next())
			require.NoError(t, it.Err())
		})
	}
}
