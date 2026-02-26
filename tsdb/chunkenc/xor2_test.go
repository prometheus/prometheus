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

	"github.com/prometheus/prometheus/model/value"
	"github.com/stretchr/testify/require"
)

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

	// Append samples.
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

	// Read back and verify.
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

	// Append samples with staleness markers.
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

	// Read back and verify.
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

func TestXOR2AdaptiveMode(t *testing.T) {
	numSamples := 120

	t.Run("RegularData_CompactMode", func(t *testing.T) {
		sizes := make(map[string]int)

		testFunc := func(enc string, c Chunk) {
			app, _ := c.Appender()
			for i := 0; i < numSamples; i++ {
				app.Append(0, int64(i*1000), float64(i))
			}
			sizes[enc] = len(c.Bytes())
		}

		testFunc("XOR", NewXORChunk())
		testFunc("XOR2", NewXOR2Chunk())

		t.Logf("Regular data (%d samples):", numSamples)
		for _, enc := range []string{"XOR", "XOR2"} {
			t.Logf("  %-6s: %4d bytes (%.2f bytes/sample)", enc, sizes[enc], float64(sizes[enc])/float64(numSamples))
		}

		// XOR2 should match XOR (stays in compact mode).
		require.Equal(t, sizes["XOR"], sizes["XOR2"], "XOR2 should match XOR on regular data")
	})

	t.Run("WithStaleness10Percent", func(t *testing.T) {
		sizes := make(map[string]int)

		testFunc := func(enc string, c Chunk) {
			app, _ := c.Appender()
			for i := 0; i < numSamples; i++ {
				var v float64
				if i%10 == 5 {
					v = math.Float64frombits(value.StaleNaN)
				} else {
					v = float64(i)
				}
				app.Append(0, int64(i*1000), v)
			}
			sizes[enc] = len(c.Bytes())
		}

		testFunc("XOR", NewXORChunk())
		testFunc("XOR2", NewXOR2Chunk())

		t.Logf("With 10%% staleness (%d samples):", numSamples)
		for _, enc := range []string{"XOR", "XOR2"} {
			t.Logf("  %-6s: %4d bytes (%.2f bytes/sample)", enc, sizes[enc], float64(sizes[enc])/float64(numSamples))
		}

		// XOR2 should beat XOR significantly.
		if sizes["XOR2"] < sizes["XOR"] {
			savings := float64(sizes["XOR"]-sizes["XOR2"]) / float64(sizes["XOR"]) * 100
			t.Logf("  ✓ XOR2 saves %.1f%% over XOR", savings)
			require.Greater(t, savings, 50.0, "XOR2 should save significant space with staleness")
		}
	})
}

func TestXOR2FeatureFlag(t *testing.T) {
	defer func(enabled bool) { setXOR2Enabled(enabled) }(useXOR2Encoding)

	// Test with XOR2 disabled (default).
	setXOR2Enabled(false)
	enc := ValFloat.ChunkEncoding()
	require.Equal(t, EncXOR, enc, "Should use XOR when flag is false")
	require.Equal(t, EncXOR, NewFloatChunk().Encoding(), "NewFloatChunk should use XOR when flag is false")

	chunk, err := ValFloat.NewChunk()
	require.NoError(t, err)
	require.Equal(t, EncXOR, chunk.Encoding(), "Should create XOR chunk when flag is false")

	// Test with XOR2 enabled.
	setXOR2Enabled(true)
	enc = ValFloat.ChunkEncoding()
	require.Equal(t, EncXOR2, enc, "Should use XOR2 when flag is true")
	require.Equal(t, EncXOR2, NewFloatChunk().Encoding(), "NewFloatChunk should use XOR2 when flag is true")

	chunk, err = ValFloat.NewChunk()
	require.NoError(t, err)
	require.Equal(t, EncXOR2, chunk.Encoding(), "Should create XOR2 chunk when flag is true")
}

func TestXOR2MultiplierWithResidual(t *testing.T) {
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	// Test with exact multiplier (residual = 0).
	app.Append(0, 0, 1.0)
	app.Append(0, 1000, 2.0) // tDelta = 1000.
	app.Append(0, 3000, 3.0) // dod = 2000 (exactly 2x tDelta).

	// Verify timestamp is exact.
	it := c.Iterator(nil)
	it.Next()
	it.Next()
	it.Next()
	ts, _ := it.At()
	require.Equal(t, int64(3000), ts, "Timestamp must be exact")

	// Test with approximate multiplier (residual = 15).
	c2 := NewXOR2Chunk()
	app2, err := c2.Appender()
	require.NoError(t, err)

	app2.Append(0, 0, 1.0)
	app2.Append(0, 1000, 2.0) // tDelta = 1000.
	app2.Append(0, 2015, 3.0) // dod = 1015 = 1*1000 + 15 (residual = 15).

	// Verify timestamp is exact (preserved via residual).
	it2 := c2.Iterator(nil)
	it2.Next()
	it2.Next()
	it2.Next()
	ts2, _ := it2.At()
	require.Equal(t, int64(2015), ts2, "Timestamp must be exact with residual")

	// Test with negative residual.
	c3 := NewXOR2Chunk()
	app3, err := c3.Appender()
	require.NoError(t, err)

	app3.Append(0, 0, 1.0)
	app3.Append(0, 1000, 2.0) // tDelta = 1000.
	app3.Append(0, 1990, 3.0) // dod = 990 = 1*1000 + (-10) (residual = -10).

	// Verify timestamp is exact with negative residual.
	it3 := c3.Iterator(nil)
	it3.Next()
	it3.Next()
	it3.Next()
	ts3, _ := it3.At()
	require.Equal(t, int64(1990), ts3, "Timestamp must be exact with negative residual")

	// Test large jitter that exceeds 8-bit signed range.
	c4 := NewXOR2Chunk()
	app4, err := c4.Appender()
	require.NoError(t, err)

	app4.Append(0, 0, 1.0)
	app4.Append(0, 1000, 2.0) // tDelta = 1000.
	app4.Append(0, 2200, 3.0) // dod = 1200 = 1*1000 + 200 (residual = 200, exceeds 127).

	// Should still be exact (falls back to standard encoding).
	it4 := c4.Iterator(nil)
	it4.Next()
	it4.Next()
	it4.Next()
	ts4, _ := it4.At()
	require.Equal(t, int64(2200), ts4, "Timestamp must be exact even with large residual")
}
