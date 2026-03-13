// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunkenc

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/value"
)

func BenchmarkXor2Write(b *testing.B) {
	samples := make([]struct {
		t int64
		v float64
	}, 120)
	for i := range samples {
		samples[i].t = int64(i) * 1000
		samples[i].v = float64(i) + float64(i)/10 + float64(i)/100 + float64(i)/1000
	}

	b.ReportAllocs()

	for b.Loop() {
		c := NewXOR2Chunk()
		app, _ := c.Appender()
		for _, s := range samples {
			app.Append(0, s.t, s.v)
		}
	}
}

func BenchmarkXor2Read(b *testing.B) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(b, err)
	for i := int64(0); i < 120*1000; i += 1000 {
		app.Append(0, i, float64(i)+float64(i)/10+float64(i)/100+float64(i)/1000)
	}

	b.ReportAllocs()

	var it Iterator
	for b.Loop() {
		var ts int64
		var v float64
		it = c.Iterator(it)
		for it.Next() != ValNone {
			ts, v = it.At()
		}
		_, _ = ts, v
	}
}

func TestXOR2Basic(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	samples := []struct {
		t int64
		v float64
	}{
		{1000, 1.0},
		{2000, 2.0},
		{3000, 3.0},
		{4000, 4.0},
		{5000, 5.0},
	}

	for _, s := range samples {
		app.Append(0, s.t, s.v)
	}

	it := c.Iterator(nil)
	for _, expected := range samples {
		require.Equal(t, ValFloat, it.Next())
		ts, v := it.At()
		require.Equal(t, expected.t, ts)
		require.Equal(t, expected.v, v)
	}
	require.Equal(t, ValNone, it.Next())
}

func TestXOR2WithStaleness(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	samples := []struct {
		t     int64
		v     float64
		stale bool
	}{
		{1000, 1.0, false},
		{2000, 2.0, false},
		{3000, math.Float64frombits(value.StaleNaN), true},
		{4000, 4.0, false},
		{5000, math.Float64frombits(value.StaleNaN), true},
		{6000, 6.0, false},
	}

	for _, s := range samples {
		app.Append(0, s.t, s.v)
	}

	it := c.Iterator(nil)
	for _, expected := range samples {
		require.Equal(t, ValFloat, it.Next())
		ts, v := it.At()
		require.Equal(t, expected.t, ts)
		if expected.stale {
			require.True(t, value.IsStaleNaN(v), "Expected stale NaN at ts=%d", ts)
		} else {
			require.Equal(t, expected.v, v)
		}
	}
	require.Equal(t, ValNone, it.Next())
}

func TestXOR2StaleWithDodNonZero(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	// Stale NaN samples where the timestamp dod is non-zero, exercising the
	// `111` value encoding path inside writeVDelta.
	samples := []struct {
		t     int64
		v     float64
		stale bool
	}{
		{1000, 1.0, false},
		{2000, 2.0, false},
		// dod = (1050 - 1000) - (2000 - 1000) = 50 - 1000 = -950: stale with dod≠0.
		{3050, math.Float64frombits(value.StaleNaN), true},
		{4050, 4.0, false},
		{5050, 5.0, false},
	}

	for _, s := range samples {
		app.Append(0, s.t, s.v)
	}

	it := c.Iterator(nil)
	for _, expected := range samples {
		require.Equal(t, ValFloat, it.Next())
		ts, v := it.At()
		require.Equal(t, expected.t, ts)
		if expected.stale {
			require.True(t, value.IsStaleNaN(v), "Expected stale NaN at ts=%d", ts)
		} else {
			require.Equal(t, expected.v, v)
		}
	}
	require.Equal(t, ValNone, it.Next())
}

func TestXOR2IrregularTimestamps(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	// Timestamps with dod values spanning multiple encoding ranges.
	timestamps := []int64{
		1000, 2000, 3000,
		// dod in 13-bit range.
		3050, 4050, 5050,
		// dod in 20-bit range (large jitter).
		5050 + 100000, 5050 + 200000, 5050 + 300000,
		// Back to regular.
		5050 + 301000,
	}
	for _, ts := range timestamps {
		app.Append(0, ts, 1.0)
	}

	it := c.Iterator(nil)
	for _, expected := range timestamps {
		require.Equal(t, ValFloat, it.Next())
		ts, _ := it.At()
		require.Equal(t, expected, ts)
	}
	require.Equal(t, ValNone, it.Next())
}

func TestXOR2LargeDod(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	// Force the 64-bit escape path with a very large dod.
	timestamps := []int64{0, 1000, 2000, 2000 + (1 << 20)}
	for _, ts := range timestamps {
		app.Append(0, ts, 1.0)
	}

	it := c.Iterator(nil)
	for _, expected := range timestamps {
		require.Equal(t, ValFloat, it.Next())
		ts, _ := it.At()
		require.Equal(t, expected, ts)
	}
	require.Equal(t, ValNone, it.Next())
}

func TestXOR2ChunkST(t *testing.T) {
	testChunkSTHandling(t, ValFloat, func() Chunk {
		return NewXOR2Chunk()
	})
}

func TestXOR2Chunk_MoreThan127Samples(t *testing.T) {
	const afterMax = maxFirstSTChangeOn + 3
	t.Run("zero ST", func(t *testing.T) {
		chunk := NewXOR2Chunk()
		app, err := chunk.Appender()
		require.NoError(t, err)
		for i := range afterMax {
			app.Append(0, int64(i*10+1), float64(i)*1.5)
		}

		it := chunk.Iterator(nil)
		for i := range afterMax {
			require.Equal(t, ValFloat, it.Next())
			st := it.AtST()
			ts, v := it.At()
			require.Equal(t, int64(0), st)
			require.Equal(t, int64(i*10+1), ts)
			require.Equal(t, float64(i)*1.5, v)
		}

		require.Equal(t, ValNone, it.Next())
		require.NoError(t, it.Err())
	})

	t.Run("non-zero ST after 127", func(t *testing.T) {
		chunk := NewXOR2Chunk()
		app, err := chunk.Appender()
		require.NoError(t, err)
		for i := range afterMax {
			st := int64(0)
			if i == afterMax-1 {
				st = int64((afterMax - 1) * 10)
			}
			app.Append(st, int64(i*10+1), float64(i)*1.5)
		}

		it := chunk.Iterator(nil)
		for i := range afterMax {
			require.Equal(t, ValFloat, it.Next())
			st := it.AtST()
			ts, v := it.At()
			if i == afterMax-1 {
				require.Equal(t, int64((afterMax-1)*10), st)
			} else {
				require.Equal(t, int64(0), st)
			}
			require.Equal(t, int64(i*10+1), ts)
			require.Equal(t, float64(i)*1.5, v)
		}

		require.Equal(t, ValNone, it.Next())
		require.NoError(t, it.Err())
	})
}
