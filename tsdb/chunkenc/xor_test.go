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
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkXorRead(b *testing.B) {
	c := NewXORChunk()
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

func TestXOR2RoundTrip(t *testing.T) {
	// Test that XOR2 encoding correctly round-trips data.
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	type sample struct {
		t int64
		v float64
	}
	var exp []sample

	ts := int64(1000)
	v := 123.456
	for i := range 120 {
		ts += 15000 + int64(i%10-5) // 15s with small jitter.
		v += float64(i%20 - 10)
		app.Append(0, ts, v)
		exp = append(exp, sample{t: ts, v: v})
	}

	require.Equal(t, 120, c.NumSamples())
	require.Equal(t, EncXOR2, c.Encoding())

	// Verify iteration.
	it := c.Iterator(nil)
	var got []sample
	for it.Next() == ValFloat {
		ts, v := it.At()
		got = append(got, sample{t: ts, v: v})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, got)
}

func TestXOR2VsXORCorrectness(t *testing.T) {
	// Verify that XOR and XOR2 produce the same sample values.
	xorChunk := NewXORChunk()
	xor2Chunk := NewXOR2Chunk()

	xorApp, err := xorChunk.Appender()
	require.NoError(t, err)
	xor2App, err := xor2Chunk.Appender()
	require.NoError(t, err)

	ts := int64(1000000)
	v := 42.0
	for range 120 {
		ts += 15000
		v += 1.5
		xorApp.Append(0, ts, v)
		xor2App.Append(0, ts, v)
	}

	xorIt := xorChunk.Iterator(nil)
	xor2It := xor2Chunk.Iterator(nil)

	for {
		xorVal := xorIt.Next()
		xor2Val := xor2It.Next()
		require.Equal(t, xorVal, xor2Val)
		if xorVal == ValNone {
			break
		}
		xorT, xorV := xorIt.At()
		xor2T, xor2V := xor2It.At()
		require.Equal(t, xorT, xor2T)
		require.Equal(t, xorV, xor2V)
	}
	require.NoError(t, xorIt.Err())
	require.NoError(t, xor2It.Err())
}

func TestXOR2SmallerForJitteryTimestamps(t *testing.T) {
	// XOR2 should produce smaller chunks when timestamps have small jitter,
	// because varbit_int encoding has finer-grained bit buckets than XOR's
	// coarse 4-bucket approach.
	xorChunk := NewXORChunk()
	xor2Chunk := NewXOR2Chunk()

	xorApp, err := xorChunk.Appender()
	require.NoError(t, err)
	xor2App, err := xor2Chunk.Appender()
	require.NoError(t, err)

	ts := int64(1000000)
	v := 42.0
	for i := range 120 {
		// Small jitter: delta-of-delta will be in the range [-5, 5].
		ts += 15000 + int64(i%11-5)
		v += 1.5
		xorApp.Append(0, ts, v)
		xor2App.Append(0, ts, v)
	}

	// XOR2 should be smaller or equal for this pattern.
	require.LessOrEqual(t, len(xor2Chunk.Bytes()), len(xorChunk.Bytes()),
		"XOR2 chunk should be smaller or equal to XOR for jittery timestamps")
}

func TestXOR2FromData(t *testing.T) {
	// Test that FromData correctly creates an XOR2Chunk.
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	app.Append(0, 1000, 1.0)
	app.Append(0, 2000, 2.0)
	app.Append(0, 3000, 3.0)

	// Create a chunk from the raw bytes.
	c2, err := FromData(EncXOR2, c.Bytes())
	require.NoError(t, err)
	require.Equal(t, EncXOR2, c2.Encoding())
	require.Equal(t, 3, c2.NumSamples())

	it := c2.Iterator(nil)
	require.Equal(t, ValFloat, it.Next())
	ts, v := it.At()
	require.Equal(t, int64(1000), ts)
	require.Equal(t, 1.0, v)

	require.Equal(t, ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(2000), ts)
	require.Equal(t, 2.0, v)

	require.Equal(t, ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(3000), ts)
	require.Equal(t, 3.0, v)

	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}

func TestXOR2Appender(t *testing.T) {
	// Test that getting an appender from a non-empty XOR2 chunk works.
	c := NewXOR2Chunk()
	app, err := c.Appender()
	require.NoError(t, err)

	app.Append(0, 1000, 1.0)
	app.Append(0, 2000, 2.0)

	// Get a new appender from the existing chunk.
	app2, err := c.Appender()
	require.NoError(t, err)
	app2.Append(0, 3000, 3.0)

	require.Equal(t, 3, c.NumSamples())

	it := c.Iterator(nil)
	require.Equal(t, ValFloat, it.Next())
	ts, v := it.At()
	require.Equal(t, int64(1000), ts)
	require.Equal(t, 1.0, v)

	require.Equal(t, ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(2000), ts)
	require.Equal(t, 2.0, v)

	require.Equal(t, ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(3000), ts)
	require.Equal(t, 3.0, v)

	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}
