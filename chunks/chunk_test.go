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
			for range make([]struct{}, 3000) {
				c := nc(rand.Intn(1024))
				if err := testChunk(c); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func testChunk(c Chunk) error {
	app, err := c.Appender()
	if err != nil {
		return err
	}

	var exp []pair
	var (
		ts = int64(1234123324)
		v  = 1243535.123
	)
	i := 0
	for i < 3 {
		ts += int64(rand.Intn(10000) + 1)
		v = rand.Float64()
		// v += float64(100)

		// Start with a new appender every 10th sample. This emulates starting
		// appending to a partially filled chunk.
		if i%10 == 0 {
			app, err = c.Appender()
			if err != nil {
				return err
			}
		}

		err = app.Append(ts, v)
		if err != nil {
			if err == ErrChunkFull {
				break
			}
			return err
		}
		exp = append(exp, pair{t: ts, v: v})
		i++
		// fmt.Println("appended", len(c.Bytes()), c.Bytes())
	}

	it := c.Iterator()
	var res []pair
	for it.Next() {
		ts, v := it.Values()
		res = append(res, pair{t: ts, v: v})
	}
	if it.Err() != nil {
		return it.Err()
	}
	if !reflect.DeepEqual(exp, res) {
		return fmt.Errorf("unexpected result\n\ngot: %v\n\nexp: %v", res, exp)
	}
	return nil
}

func benchmarkIterator(b *testing.B, newChunk func(int) Chunk) {
	var (
		t = int64(1234123324)
		v = 1243535.123
	)
	var exp []pair
	for i := 0; i < b.N; i++ {
		t += int64(rand.Intn(10000) + 1)
		// v = rand.Float64()
		v += float64(100)
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
		// v = rand.Float64()
		v += float64(100)
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
