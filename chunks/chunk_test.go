package chunks

import (
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

type pair struct {
	t int64
	v float64
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
