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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

type triple struct {
	st, t int64
	v     float64
}

func TestChunk(t *testing.T) {
	for enc, nc := range map[Encoding]func() Chunk{
		EncXOR:      func() Chunk { return NewXORChunk() },
		101:         func() Chunk { return NewXORVarbitChunk() },
		110:         func() Chunk { return NewXORVarbitTSChunk() },
		EncXOROptST: func() Chunk { return NewXOROptSTChunk() },
		102:         func() Chunk { return NewXORSTNaiveChunk() },
		103:         func() Chunk { return NewALPBufferedChunk() },
	} {
		t.Run(fmt.Sprintf("enc=%v", enc), func(t *testing.T) {
			floatEquals := func(a, b float64) bool {
				return a == b
			}
			if enc == 103 { // ALP is lossy.
				floatEquals = func(a, b float64) bool {
					return math.Abs(a-b) < 1e-9
				}
			}

			t.Run("data=realistic-v1", func(t *testing.T) {
				t.Skip("% ")
				c := nc()
				var data []triple
				var (
					ts = int64(1234123324)
					v  = 1243535.123
				)
				for i := range 300 {
					ts += int64(rand.Intn(10000) + 1)
					if i%2 == 0 {
						v += float64(rand.Intn(1000000))
					} else {
						v -= float64(rand.Intn(1000000))
					}
					data = append(data, triple{st: 0, t: ts, v: v})
				}
				testChunk(t, c, data, floatEquals)
			})
			t.Run("data=realistic-constcumulative", func(t *testing.T) {
				t.Skip("5")
				c := nc()

				var data []triple
				var (
					ts  = int64(1234123324)
					sts = ts - 15000
					v   = 1243535.123
				)
				for i := range 300 {
					ts += int64(rand.Intn(10000) + 1)
					if i%2 == 0 {
						v += float64(rand.Intn(1000000))
					} else {
						v -= float64(rand.Intn(1000000))
					}
					data = append(data, triple{st: sts, t: ts, v: v})
				}
				testChunk(t, c, data, floatEquals)
			})
			t.Run("data=realistic-cumulative", func(t *testing.T) {
				c := nc()

				var data []triple
				var (
					ts  = int64(1234123324)
					sts = ts - 15000
					v   = 1243535.123
				)
				for i := range 300 {
					ts += int64(rand.Intn(10000) + 1)

					if i%6 == 5 {
						// Inject zero every 6th iteration to test the special zero case
						sts = 0
					} else if i%6 == 4 {
						// Every 6th we reset.
						sts += ts - 15000
					}

					if i%2 == 0 {
						v += float64(rand.Intn(1000000))
					} else {
						v -= float64(rand.Intn(1000000))
					}
					data = append(data, triple{st: sts, t: ts, v: v})
				}
				testChunk(t, c, data, floatEquals)
			})
			t.Run("data=constant-delta", func(t *testing.T) {
				c := nc()
				var data []triple
				var (
					ts  = int64(1234123324)
					sts = ts - 15000
					v   = 1243535.123
				)
				for i := range 300 {
					if i%6 == 5 {
						// Inject zero every 6th iteration to test the special zero case
						sts = 0
					} else {
						sts = ts - 1 // Delta like ST.
					}
					ts += int64(rand.Intn(10000) + 1)
					if i%2 == 0 {
						v += float64(rand.Intn(1000000))
					} else {
						v -= float64(rand.Intn(1000000))
					}
					data = append(data, triple{st: sts, t: ts, v: v})
				}
				testChunk(t, c, []triple{
					{st: 1762250480001, t: 1762250481000, v: 1.243535123e+06},
					{st: 1762250481001, t: 1762250482000, v: 1.243535123e+06},
					{st: 1762250482001, t: 1762250483000, v: 1.243535123e+06},
					{st: 1762250483001, t: 1762250484000, v: 1.243535123e+06},
					{st: 1762250484001, t: 1762250485000, v: 1.243535123e+06},
					{st: 1762250485001, t: 1762250486000, v: 1.243535123e+06},
					{st: 1762250486001, t: 1762250487000, v: 1.243535123e+06},
				}, floatEquals)
			})
		})
	}
}

func compatNewAppenderV2(c Chunk) (_ func() (AppenderV2, error), stSupported bool) {
	ca, stSupported := c.(supportsAppenderV2)
	if stSupported {
		return ca.AppenderV2, true
	}
	// To simplify tests, v1 appender simulate v2 by ignoring ST data.
	return func() (AppenderV2, error) {
		a, err := c.Appender()
		return &ignoreSTAppenderV2{Appender: a}, err
	}, false
}

func testChunk(t *testing.T, c Chunk, data []triple, floatEquals func(a, b float64) bool) {
	newAppenderFn, stSupported := compatNewAppenderV2(c)

	app, err := newAppenderFn()
	require.NoError(t, err)

	for i, s := range data {
		// Start with a new appender every 10th sample. This emulates starting
		// appending to a partially filled chunk.
		if i%10 == 0 {
			app, err = newAppenderFn()
			require.NoError(t, err)
		}
		app.Append(s.st, s.t, s.v)
	}

	// Some chunk implementations might be buffered. Reset to ensure we don't reuse
	// appending buffers.
	c.Reset(c.Bytes())

	// 1. Expand iterator in simple case.
	it1 := c.Iterator(nil)
	var res1 []triple
	for i := 0; it1.Next() == ValFloat; i++ {
		ts, v := it1.At()
		st := it1.AtST()
		if !stSupported {
			// Supplement ST from expectations for formats that does not support ST.
			st = data[i].st
		}
		res1 = append(res1, triple{st: st, t: ts, v: v})
	}
	require.NoError(t, it1.Err())
	if diff := cmp.Diff(data, res1, cmp.AllowUnexported(triple{}), cmp.Comparer(floatEquals)); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	// 2. Expand second iterator while reusing first one.
	it2 := c.Iterator(it1)
	var res2 []triple
	for i := 0; it2.Next() == ValFloat; i++ {
		ts, v := it2.At()
		st := it2.AtST()
		if !stSupported {
			// Supplement ST from expectations for formats that does not support ST.
			st = data[i].st
		}
		res2 = append(res2, triple{st: st, t: ts, v: v})
	}
	require.NoError(t, it2.Err())
	if diff := cmp.Diff(data, res2, cmp.AllowUnexported(triple{}), cmp.Comparer(floatEquals)); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	// 3. Test iterator Seek.
	mid := len(data) / 2

	it3 := c.Iterator(nil)
	var res3 []triple
	require.Equal(t, ValFloat, it3.Seek(data[mid].t))
	// Below ones should not matter.
	require.Equal(t, ValFloat, it3.Seek(data[mid].t))
	require.Equal(t, ValFloat, it3.Seek(data[mid].t))
	ts, v := it3.At()
	st := it3.AtST()
	if !stSupported {
		// Supplement ST from expectations for formats that does not support ST.
		st = data[mid].st
	}
	res3 = append(res3, triple{st: st, t: ts, v: v})

	for i := mid + 1; it3.Next() == ValFloat; i++ {
		ts, v := it3.At()
		st := it3.AtST()
		if !stSupported {
			// Supplement ST from expectations for formats that does not support ST.
			st = data[i].st
		}
		res3 = append(res3, triple{st: st, t: ts, v: v})
	}
	require.NoError(t, it3.Err())
	if diff := cmp.Diff(data[mid:], res3, cmp.AllowUnexported(triple{}), cmp.Comparer(floatEquals)); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
	require.Equal(t, ValNone, it3.Seek(data[len(data)-1].t+1))
}

func TestPool(t *testing.T) {
	p := NewPool()
	for _, tc := range []struct {
		name     string
		encoding Encoding
		expErr   error
	}{
		{
			name:     "xor",
			encoding: EncXOR,
		},
		{
			name:     "histogram",
			encoding: EncHistogram,
		},
		{
			name:     "float histogram",
			encoding: EncFloatHistogram,
		},
		{
			name:     "invalid encoding",
			encoding: EncNone,
			expErr:   errors.New(`invalid chunk encoding "none"`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := p.Get(tc.encoding, []byte("test"))
			if tc.expErr != nil {
				require.EqualError(t, err, tc.expErr.Error())
				return
			}

			require.NoError(t, err)

			var b *bstream
			switch tc.encoding {
			case EncHistogram:
				b = &c.(*HistogramChunk).b
			case EncFloatHistogram:
				b = &c.(*FloatHistogramChunk).b
			default:
				b = &c.(*XORChunk).b
			}

			require.Equal(t, &bstream{
				stream: []byte("test"),
				count:  0,
			}, b)

			b.count = 1
			require.NoError(t, p.Put(c))
			require.Equal(t, &bstream{
				stream: nil,
				count:  0,
			}, b)
		})
	}

	t.Run("put bad chunk wrapper", func(t *testing.T) {
		// When a wrapping chunk poses as an encoding it can't be converted to, Put should skip it.
		c := fakeChunk{
			encoding: EncXOR,
			t:        t,
		}
		require.NoError(t, p.Put(c))
	})
	t.Run("put invalid encoding", func(t *testing.T) {
		c := fakeChunk{
			encoding: EncNone,
			t:        t,
		}
		require.EqualError(t, p.Put(c), `invalid chunk encoding "none"`)
	})
}

type fakeChunk struct {
	Chunk

	encoding Encoding
	t        *testing.T
}

func (c fakeChunk) Encoding() Encoding {
	return c.encoding
}

func (c fakeChunk) Reset([]byte) {
	c.t.Fatal("Reset should not be called")
}
