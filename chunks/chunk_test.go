package chunks

import (
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type pair struct {
	t int64
	v float64
}

func TestChunk(t *testing.T) {
	for enc, nc := range map[Encoding]func(sz int) Chunk{
		EncXOR: func(sz int) Chunk { return NewXORChunk(sz) },
	} {
		t.Run(fmt.Sprintf("%s", enc), func(t *testing.T) {
			for range make([]struct{}, 10000) {
				c := nc(rand.Intn(512))
				testChunk(t, c)
			}
		})
	}
}

func testChunk(t *testing.T, c Chunk) {
	app, err := c.Appender()
	if err != nil {
		t.Fatal(err)
	}

	var exp []pair
	var (
		ts = int64(1234123324)
		v  = 1243535.123
	)
	for {
		ts += int64(rand.Intn(10000) + 1)
		v = rand.Float64()

		err := app.Append(ts, v)
		if err != nil {
			if err == ErrChunkFull {
				break
			}
			t.Fatal(err)
		}
		exp = append(exp, pair{t: ts, v: v})
	}

	it := c.Iterator()
	var res []pair
	for it.Next() {
		ts, v := it.Values()
		res = append(res, pair{t: ts, v: v})
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	if !reflect.DeepEqual(exp, res) {
		t.Fatalf("unexpected result\n\ngot: %v\n\nexp: %v", res, exp)
	}
}

func benchmarkIterator(b *testing.B, newChunk func(int) Chunk) {
	var (
		t = int64(1234123324)
		v = 1243535.123
	)
	var exp []pair
	for i := 0; i < b.N; i++ {
		t += int64(rand.Intn(10000) + 1)
		v = rand.Float64()
		exp = append(exp, pair{t: t, v: v})
	}

	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk(1024)

		a, err := c.Appender()
		if err != nil {
			b.Fatalf("get appender: %s", err)
		}
		for _, p := range exp {
			if err := a.Append(p.t, p.v); err == ErrChunkFull {
				break
			} else if err != nil {
				b.Fatal(err)
			}
			i++
		}
		chunks = append(chunks, c)
	}

	b.ReportAllocs()
	b.ResetTimer()

	fmt.Println("num", b.N, "created chunks", len(chunks))

	res := make([]float64, 0, 1024)

	for i := 0; i < len(chunks); i++ {
		c := chunks[i]
		it := c.Iterator()

		for it.Next() {
			_, v := it.Values()
			res = append(res, v)
		}
		if it.Err() != io.EOF {
			require.NoError(b, it.Err())
		}
		res = res[:0]
	}
}

func BenchmarkXORIterator(b *testing.B) {
	benchmarkIterator(b, func(sz int) Chunk {
		return NewXORChunk(sz)
	})
}

func BenchmarkXORAppender(b *testing.B) {
	benchmarkAppender(b, func(sz int) Chunk {
		return NewXORChunk(sz)
	})
}

func benchmarkAppender(b *testing.B, newChunk func(int) Chunk) {
	var (
		t = int64(1234123324)
		v = 1243535.123
	)
	var exp []pair
	for i := 0; i < b.N; i++ {
		t += int64(rand.Intn(10000) + 1)
		v = rand.Float64()
		exp = append(exp, pair{t: t, v: v})
	}

	b.ReportAllocs()
	b.ResetTimer()

	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk(1024)

		a, err := c.Appender()
		if err != nil {
			b.Fatalf("get appender: %s", err)
		}
		for _, p := range exp {
			if err := a.Append(p.t, p.v); err == ErrChunkFull {
				break
			} else if err != nil {
				b.Fatal(err)
			}
			i++
		}
		chunks = append(chunks, c)
	}

	fmt.Println("num", b.N, "created chunks", len(chunks))
}
