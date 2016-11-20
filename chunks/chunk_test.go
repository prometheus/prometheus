package chunks

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestRawChunkAppend(t *testing.T) {
	c := newRawChunk(1, 'a')
	require.NoError(t, c.append(nil))
	require.Error(t, c.append([]byte("t")))

	c = newRawChunk(5, 'a')
	require.NoError(t, c.append([]byte("test")))
	require.Error(t, c.append([]byte("x")))
	require.Equal(t, rawChunk{d: []byte("atest"), l: 5}, c)
	require.Equal(t, []byte("atest"), c.Data())
}

func TestPlainAppender(t *testing.T) {
	c := NewPlainChunk(3*16 + 1)
	a := c.Appender()

	require.NoError(t, a.Append(1, 1))
	require.NoError(t, a.Append(2, 2))
	require.NoError(t, a.Append(3, 3))
	require.Equal(t, ErrChunkFull, a.Append(4, 4))

	exp := []byte{byte(EncPlain)}
	b := make([]byte, 8)
	for i := 1; i <= 3; i++ {
		binary.LittleEndian.PutUint64(b, uint64(i))
		exp = append(exp, b...)
		binary.LittleEndian.PutUint64(b, math.Float64bits(float64(i)))
		exp = append(exp, b...)
	}
	require.Equal(t, exp, c.Data())
}

func TestPlainIterator(t *testing.T) {
	c := NewPlainChunk(100*16 + 1)
	a := c.Appender()

	var exp []model.SamplePair
	for i := 0; i < 100; i++ {
		exp = append(exp, model.SamplePair{
			Timestamp: model.Time(i * 2),
			Value:     model.SampleValue(i * 2),
		})
		require.NoError(t, a.Append(model.Time(i*2), model.SampleValue(i*2)))
	}

	it := c.Iterator()

	var res1 []model.SamplePair
	for s, ok := it.Seek(0); ok; s, ok = it.Next() {
		res1 = append(res1, s)
	}
	require.Equal(t, io.EOF, it.Err())
	require.Equal(t, exp, res1)

	var res2 []model.SamplePair
	for s, ok := it.Seek(11); ok; s, ok = it.Next() {
		res2 = append(res2, s)
	}
	require.Equal(t, io.EOF, it.Err())
	require.Equal(t, exp[6:], res2)
}

func benchmarkIterator(b *testing.B, newChunk func(int) Chunk) {
	var (
		baseT = model.Now()
		baseV = 1243535
	)
	var exp []model.SamplePair
	for i := 0; i < b.N; i++ {
		baseT += model.Time(rand.Intn(10000))
		baseV += rand.Intn(10000)
		exp = append(exp, model.SamplePair{
			Timestamp: baseT,
			Value:     model.SampleValue(baseV),
		})
	}
	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk(1024)
		a := c.Appender()
		for i < b.N {
			if err := a.Append(exp[i].Timestamp, exp[i].Value); err == ErrChunkFull {
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
	fmt.Println("num", b.N)

	res := make([]model.SamplePair, 0, 1024)
	for i := 0; i < len(chunks); i++ {
		c := chunks[i]
		it := c.Iterator()

		for s, ok := it.First(); ok; s, ok = it.Next() {
			res = append(res, s)
		}
		if it.Err() != io.EOF {
			require.NoError(b, it.Err())
		}
		res = res[:0]
	}
}

func BenchmarkPlainIterator(b *testing.B) {
	benchmarkIterator(b, func(sz int) Chunk {
		return NewPlainChunk(sz)
	})
}

func BenchmarkDoubleDeltaIterator(b *testing.B) {
	benchmarkIterator(b, func(sz int) Chunk {
		return NewDoubleDeltaChunk(sz)
	})
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
		baseT = model.Now()
		baseV = 1243535
	)
	var exp []model.SamplePair
	for i := 0; i < b.N; i++ {
		baseT += model.Time(rand.Intn(10000))
		baseV += rand.Intn(10000)
		exp = append(exp, model.SamplePair{
			Timestamp: baseT,
			Value:     model.SampleValue(baseV),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	var chunks []Chunk
	for i := 0; i < b.N; {
		c := newChunk(1024)
		a := c.Appender()
		for i < b.N {
			if err := a.Append(exp[i].Timestamp, exp[i].Value); err == ErrChunkFull {
				break
			} else if err != nil {
				b.Fatal(err)
			}
			i++
		}
		chunks = append(chunks, c)
	}
	fmt.Println("created chunks", len(chunks))
}

func BenchmarkPlainAppender(b *testing.B) {
	benchmarkAppender(b, func(sz int) Chunk {
		return NewPlainChunk(sz)
	})
}

func BenchmarkDoubleDeltaAppender(b *testing.B) {
	benchmarkAppender(b, func(sz int) Chunk {
		return NewDoubleDeltaChunk(sz)
	})
}
