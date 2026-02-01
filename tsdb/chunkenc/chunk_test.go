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
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

type triple struct {
	st, t int64
	v     float64
}

func TestChunk(t *testing.T) {
	testcases := []struct {
		encoding   Encoding
		supportsST bool
		factory    func() Chunk
	}{
		{encoding: EncXOR, supportsST: false, factory: func() Chunk { return NewXORChunk() }},
		{encoding: EncXORST, supportsST: true, factory: func() Chunk { return NewXORSTChunk() }},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%v", tc.encoding), func(t *testing.T) {
			for range make([]struct{}, 1) {
				c := tc.factory()
				testChunk(t, c, tc.supportsST)
			}
		})
	}
}

func testChunk(t *testing.T, c Chunk, supportsST bool) {
	app, err := c.Appender()
	require.NoError(t, err)

	var exp []triple
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

		// Start with a new appender every 10th sample. This emulates starting
		// appending to a partially filled chunk.
		if i%10 == 0 {
			app, err = c.Appender()
			require.NoError(t, err)
		}

		app.Append(ts-100, ts, v)
		expST := int64(0)
		if supportsST {
			expST = ts - 100
		}
		exp = append(exp, triple{st: expST, t: ts, v: v})
	}

	// 1. Expand iterator in simple case.
	it1 := c.Iterator(nil)
	var res1 []triple
	for it1.Next() == ValFloat {
		ts, v := it1.At()
		res1 = append(res1, triple{st: it1.AtST(), t: ts, v: v})
	}
	require.NoError(t, it1.Err())
	require.Equal(t, exp, res1)

	// 2. Expand second iterator while reusing first one.
	it2 := c.Iterator(it1)
	var res2 []triple
	for it2.Next() == ValFloat {
		ts, v := it2.At()
		res2 = append(res2, triple{st: it2.AtST(), t: ts, v: v})
	}
	require.NoError(t, it2.Err())
	require.Equal(t, exp, res2)

	// 3. Test iterator Seek.
	mid := len(exp) / 2

	it3 := c.Iterator(nil)
	var res3 []triple
	require.Equal(t, ValFloat, it3.Seek(exp[mid].t))
	// Below ones should not matter.
	require.Equal(t, ValFloat, it3.Seek(exp[mid].t))
	require.Equal(t, ValFloat, it3.Seek(exp[mid].t))
	ts, v = it3.At()
	res3 = append(res3, triple{st: it3.AtST(), t: ts, v: v})

	lastTs := ts
	for it3.Next() == ValFloat {
		ts, v := it3.At()
		lastTs = ts
		res3 = append(res3, triple{st: it3.AtST(), t: ts, v: v})
	}
	// Seeking to last timestamp should work and it is a no-op.
	require.Equal(t, ValFloat, it3.Seek(lastTs))
	require.NoError(t, it3.Err())
	require.Equal(t, exp[mid:], res3)
	require.Equal(t, ValNone, it3.Seek(exp[len(exp)-1].t+1))
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
			name:     "xor opt st",
			encoding: EncXORST,
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
			case EncXORST:
				b = &c.(*XorSTChunk).b
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
