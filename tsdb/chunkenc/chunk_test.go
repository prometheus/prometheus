// Copyright 2017 The Prometheus Authors
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
	"io"
	"math/rand"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

type pair struct {
	t int64
	v float64
}

func TestChunk(t *testing.T) {
	for enc, nc := range map[Encoding]func() Chunk{
		EncXOR: func() Chunk { return NewXORChunk() },
	} {
		t.Run(enc.String(), func(t *testing.T) {
			c := nc()
			exp := appendToChunk(t, c, pair{t: 1234123324, v: 1243535.123})

			t.Run("original", func(t *testing.T) {
				testChunk(t, exp, c)
			})

			clonable, ok := c.(Clonable)
			testutil.Assert(t, ok, "chunk does not implement clonable")

			clone1 := clonable.Clone()
			clone2 := clonable.Clone()

			exp2 := appendToChunk(t, clone1, exp[len(exp)-1])

			t.Run("original not changed", func(t *testing.T) {
				testChunk(t, exp, c)
			})
			t.Run("clone2 not changed", func(t *testing.T) {
				testChunk(t, exp, clone2)
			})
			t.Run("clone1 changed", func(t *testing.T) {
				testChunk(t, append(append([]pair{}, exp...), exp2...), clone1)
			})
		})
	}
}

func appendToChunk(t *testing.T, c Chunk, start pair) (exp []pair) {
	app, err := c.Appender()
	testutil.Ok(t, err)

	ts := start.t
	v := start.v
	for i := 0; i < 300; i++ {
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
			testutil.Ok(t, err)
		}

		app.Append(ts, v)
		exp = append(exp, pair{t: ts, v: v})
	}
	return exp
}

func testChunk(t *testing.T, exp []pair, c Chunk) {
	it1 := c.Iterator(nil)

	// 1. Expand iterator in simple case.
	t.Run("iterator-simple", func(t *testing.T) {
		var res1 []pair
		for it1.Next() {
			ts, v := it1.At()
			res1 = append(res1, pair{t: ts, v: v})
		}
		testutil.Ok(t, it1.Err())
		testutil.Equals(t, exp, res1)
	})

	// 2. Expand second iterator while reusing first one.
	t.Run("iterator-reuse", func(t *testing.T) {
		it2 := c.Iterator(it1)
		var res2 []pair
		for it2.Next() {
			ts, v := it2.At()
			res2 = append(res2, pair{t: ts, v: v})
		}
		testutil.Ok(t, it2.Err())
		testutil.Equals(t, exp, res2)
	})

	// 3. Test iterator Seek.
	t.Run("seek", func(t *testing.T) {
		mid := len(exp) / 2

		it3 := c.Iterator(nil)
		var res3 []pair
		testutil.Equals(t, true, it3.Seek(exp[mid].t))
		// Below ones should not matter.
		testutil.Equals(t, true, it3.Seek(exp[mid].t))
		testutil.Equals(t, true, it3.Seek(exp[mid].t))
		ts, v := it3.At()
		res3 = append(res3, pair{t: ts, v: v})

		for it3.Next() {
			ts, v := it3.At()
			res3 = append(res3, pair{t: ts, v: v})
		}
		testutil.Ok(t, it3.Err())
		testutil.Equals(t, exp[mid:], res3)
		testutil.Equals(t, false, it3.Seek(exp[len(exp)-1].t+1))
	})
}

func benchmarkIterator(b *testing.B, newChunk func() Chunk) {
	var (
		t   = int64(1234123324)
		v   = 1243535.123
		exp []pair
	)
	for i := 0; i < b.N; i++ {
		// t += int64(rand.Intn(10000) + 1)
		t += int64(1000)
		// v = rand.Float64()
		v += float64(100)
		exp = append(exp, pair{t: t, v: v})
	}

	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk()

		a, err := c.Appender()
		if err != nil {
			b.Fatalf("get appender: %s", err)
		}
		j := 0
		for _, p := range exp {
			if j > 250 {
				break
			}
			a.Append(p.t, p.v)
			i++
			j++
		}
		chunks = append(chunks, c)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.Log("num", b.N, "created chunks", len(chunks))

	res := make([]float64, 0, 1024)

	var it Iterator
	for i := 0; i < len(chunks); i++ {
		c := chunks[i]
		it := c.Iterator(it)

		for it.Next() {
			_, v := it.At()
			res = append(res, v)
		}
		if it.Err() != io.EOF {
			testutil.Ok(b, it.Err())
		}
		res = res[:0]
	}
}

func BenchmarkXORIterator(b *testing.B) {
	benchmarkIterator(b, func() Chunk {
		return NewXORChunk()
	})
}

func BenchmarkXORAppender(b *testing.B) {
	benchmarkAppender(b, func() Chunk {
		return NewXORChunk()
	})
}

func benchmarkAppender(b *testing.B, newChunk func() Chunk) {
	var (
		t = int64(1234123324)
		v = 1243535.123
	)
	var exp []pair
	for i := 0; i < b.N; i++ {
		// t += int64(rand.Intn(10000) + 1)
		t += int64(1000)
		// v = rand.Float64()
		v += float64(100)
		exp = append(exp, pair{t: t, v: v})
	}

	b.ReportAllocs()
	b.ResetTimer()

	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk()

		a, err := c.Appender()
		if err != nil {
			b.Fatalf("get appender: %s", err)
		}
		j := 0
		for _, p := range exp {
			if j > 250 {
				break
			}
			a.Append(p.t, p.v)
			i++
			j++
		}
		chunks = append(chunks, c)
	}

	fmt.Println("num", b.N, "created chunks", len(chunks))
}
