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
	"fmt"
	"math"
	"math/bits"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/value"
)

func newXOR2IteratorForPayload(t *testing.T, padding int, payload func(*bstream), setup func(*xor2Iterator)) *xor2Iterator {
	t.Helper()

	var bs bstream
	if padding > 0 {
		bs.writeBitsFast(0, padding)
	}
	payload(&bs)
	// Add tail bytes so the reader initially fills a full 64-bit buffer.
	bs.writeBitsFast(0, 64)

	it := &xor2Iterator{}
	if setup != nil {
		setup(it)
	}
	it.br = newBReader(bs.bytes())

	if padding > 0 {
		_, err := it.br.readBits(uint8(padding))
		require.NoError(t, err)
	}

	return it
}

func writeXOR2NewWindowPayload(bs *bstream, delta uint64) (leading, trailing uint8) {
	leading, trailing, sigbits := xor2DeltaWindow(delta)
	encodedSigbits := sigbits
	if sigbits == 64 {
		encodedSigbits = 0
	}

	bs.writeBitsFast(uint64(leading), 5)
	bs.writeBitsFast(uint64(encodedSigbits), 6)
	bs.writeBitsFast(delta>>trailing, int(sigbits))

	return leading, trailing
}

func xor2DeltaWindow(delta uint64) (leading, trailing, sigbits uint8) {
	leading = uint8(bits.LeadingZeros64(delta))
	trailing = uint8(bits.TrailingZeros64(delta))
	if leading >= 32 {
		leading = 31
	}

	return leading, trailing, 64 - leading - trailing
}

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

func requireXOR2Samples(t *testing.T, samples []triple) {
	t.Helper()

	chunk := NewXOR2Chunk()
	app, err := chunk.Appender()
	require.NoError(t, err)

	for _, sample := range samples {
		app.Append(sample.st, sample.t, sample.v)
	}

	it := chunk.Iterator(nil)
	for _, want := range samples {
		require.Equal(t, ValFloat, it.Next())
		require.Equal(t, want.st, it.AtST())
		ts, v := it.At()
		require.Equal(t, want.t, ts)
		require.Equal(t, want.v, v)
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
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

func TestXOR2LargeDodWithActiveST(t *testing.T) {
	requireXOR2Samples(t, []triple{
		{st: 0, t: 0, v: 1.0},
		{st: 900, t: 1000, v: 2.0},
		{st: 1000, t: 2000, v: 3.0},
		{st: 1047576, t: 1050576, v: 4.0},
	})
}

func TestXOR2ActiveSTFastPathBoundaries(t *testing.T) {
	requireXOR2Samples(t, []triple{
		{st: 0, t: 1000, v: 1.0},
		{st: 1990, t: 2000, v: 1.0},
		{st: 2986, t: 3000, v: 1.0},
		{st: 3954, t: 4000, v: 1.0},
		{st: 4698, t: 5000, v: 1.0},
	})
}

func TestXOR2EncodeJointValueUnchangedThenChanged(t *testing.T) {
	requireXOR2Samples(t, []triple{
		{st: 0, t: 1000, v: 1.0},
		{st: 0, t: 2000, v: 2.0},
		{st: 0, t: 7096, v: 2.0},
		{st: 0, t: 12192, v: 3.0},
	})
}

func TestXOR2ConstantNonZeroSTFastPath(t *testing.T) {
	requireXOR2Samples(t, []triple{
		{st: 500, t: 1000, v: 1.0},
		{st: 500, t: 2000, v: 2.0},
		{st: 500, t: 3000, v: 2.0},
		{st: 500, t: 4050, v: 2.0},
		{st: 500, t: 5100, v: 3.0},
	})
}

func TestXOR2ActiveSTDodZeroValueChange(t *testing.T) {
	requireXOR2Samples(t, []triple{
		{st: 0, t: 1000, v: 1.0},
		{st: 500, t: 2000, v: 2.0},
		{st: 500, t: 3000, v: 3.0}, // dod=0, value changed.
		{st: 500, t: 4000, v: 4.0}, // dod=0, value changed.
	})
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

// TestXOR2DecodeFunctionsAcrossPadding exercises decodeValue,
// decodeValueKnownNonZero, and decodeNewLeadingTrailing across all logical
// cases × all 64 bit-buffer alignments (padding 0..63). Padding controls the
// number of bits that precede the payload in the stream, which determines
// how many bits remain in the 64-bit read buffer when the decode function is
// called. This Cartesian product ensures both the fast path (enough bits
// buffered for a single-shot read) and the slow path (bits span a buffer
// refill) are exercised for every case.
func TestXOR2DecodeFunctionsAcrossPadding(t *testing.T) {
	const baseline = 1234.5

	type testCase struct {
		name    string
		payload func(*bstream)
		setup   func(*xor2Iterator)
		assert  func(*testing.T, *xor2Iterator)
	}

	runCases := func(t *testing.T, cases []testCase, fn func(*xor2Iterator) error) {
		t.Helper()
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				for padding := range 64 {
					t.Run(fmt.Sprintf("padding=%d", padding), func(t *testing.T) {
						it := newXOR2IteratorForPayload(t, padding, tc.payload, tc.setup)
						require.NoError(t, fn(it))
						tc.assert(t, it)
					})
				}
			})
		}
	}

	// decodeValue: `0`=unchanged, `10`=reuse window, `110`=new window, `111`=stale NaN.
	t.Run("decodeValue", func(t *testing.T) {
		reuseD := uint64(0x000ABCDE000000)
		rL, rT, rS := xor2DeltaWindow(reuseD)

		// Two new-window variants: full-width sigbits (encoded as 0) and small
		// sigbits, to cover both value-bits read paths inside decodeNewLeadingTrailing.
		newDFull := uint64(0xFEDCBA9876543211)
		nLFull, nTFull, _ := xor2DeltaWindow(newDFull)
		newDSmall := uint64(0x000ABCDE000000)
		nLSmall, nTSmall, _ := xor2DeltaWindow(newDSmall)

		runCases(t, []testCase{
			{
				name:    "unchanged",
				payload: func(bs *bstream) { bs.writeBit(zero) },
				setup:   func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					require.Equal(t, baseline, it.val)
					require.Equal(t, baseline, it.baselineV)
				},
			},
			{
				name: "reuse_window",
				payload: func(bs *bstream) {
					bs.writeBitsFast(0b10, 2)
					bs.writeBitsFast(reuseD>>rT, int(rS))
				},
				setup: func(it *xor2Iterator) {
					it.baselineV = baseline
					it.leading, it.trailing = rL, rT
				},
				assert: func(t *testing.T, it *xor2Iterator) {
					expected := math.Float64frombits(math.Float64bits(baseline) ^ reuseD)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
					require.Equal(t, rL, it.leading)
					require.Equal(t, rT, it.trailing)
				},
			},
			{
				name: "new_window_full_sigbits",
				payload: func(bs *bstream) {
					bs.writeBitsFast(0b110, 3)
					writeXOR2NewWindowPayload(bs, newDFull)
				},
				setup: func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					expected := math.Float64frombits(math.Float64bits(baseline) ^ newDFull)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
					require.Equal(t, nLFull, it.leading)
					require.Equal(t, nTFull, it.trailing)
				},
			},
			{
				name: "new_window_small_sigbits",
				payload: func(bs *bstream) {
					bs.writeBitsFast(0b110, 3)
					writeXOR2NewWindowPayload(bs, newDSmall)
				},
				setup: func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					expected := math.Float64frombits(math.Float64bits(baseline) ^ newDSmall)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
					require.Equal(t, nLSmall, it.leading)
					require.Equal(t, nTSmall, it.trailing)
				},
			},
			{
				name:    "stale_nan",
				payload: func(bs *bstream) { bs.writeBitsFast(0b111, 3) },
				setup:   func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					require.True(t, value.IsStaleNaN(it.val))
					require.Equal(t, baseline, it.baselineV)
				},
			},
		}, (*xor2Iterator).decodeValue)
	})

	// decodeValueKnownNonZero: `0`=reuse window, `1`=new window.
	// The new_window case uses real leading/trailing (not 0xff) so that sz is
	// small enough for the fast path (valid >= 1+sz) to be reached with ctrlBit=1.
	t.Run("decodeValueKnownNonZero", func(t *testing.T) {
		delta := uint64(0x000ABCDE000000)
		dL, dT, dS := xor2DeltaWindow(delta)

		runCases(t, []testCase{
			{
				name: "reuse_window",
				payload: func(bs *bstream) {
					bs.writeBit(zero)
					bs.writeBitsFast(delta>>dT, int(dS))
				},
				setup: func(it *xor2Iterator) {
					it.baselineV = baseline
					it.leading, it.trailing = dL, dT
				},
				assert: func(t *testing.T, it *xor2Iterator) {
					expected := math.Float64frombits(math.Float64bits(baseline) ^ delta)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
				},
			},
			{
				name: "new_window",
				payload: func(bs *bstream) {
					bs.writeBit(one)
					writeXOR2NewWindowPayload(bs, delta)
				},
				setup: func(it *xor2Iterator) {
					it.baselineV = baseline
					it.leading, it.trailing = dL, dT
				},
				assert: func(t *testing.T, it *xor2Iterator) {
					expected := math.Float64frombits(math.Float64bits(baseline) ^ delta)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
					require.Equal(t, dL, it.leading)
					require.Equal(t, dT, it.trailing)
				},
			},
		}, (*xor2Iterator).decodeValueKnownNonZero)
	})

	// decodeNewLeadingTrailing: exercises the 11-bit header fast path, the
	// value-bits fast path (small sigbits), and full-width sigbits (encoded as 0).
	t.Run("decodeNewLeadingTrailing", func(t *testing.T) {
		smallD := uint64(0x000ABCDE000000)
		sL, sT, _ := xor2DeltaWindow(smallD)
		fullD := uint64(0xFEDCBA9876543211)
		fL, fT, _ := xor2DeltaWindow(fullD)

		runCases(t, []testCase{
			{
				name:    "small_sigbits",
				payload: func(bs *bstream) { writeXOR2NewWindowPayload(bs, smallD) },
				setup:   func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					require.Equal(t, sL, it.leading)
					require.Equal(t, sT, it.trailing)
					expected := math.Float64frombits(math.Float64bits(baseline) ^ smallD)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
				},
			},
			{
				name:    "full_width_sigbits",
				payload: func(bs *bstream) { writeXOR2NewWindowPayload(bs, fullD) },
				setup:   func(it *xor2Iterator) { it.baselineV = baseline },
				assert: func(t *testing.T, it *xor2Iterator) {
					require.Equal(t, fL, it.leading)
					require.Equal(t, fT, it.trailing)
					expected := math.Float64frombits(math.Float64bits(baseline) ^ fullD)
					require.Equal(t, expected, it.val)
					require.Equal(t, expected, it.baselineV)
				},
			},
		}, (*xor2Iterator).decodeNewLeadingTrailing)
	})
}
