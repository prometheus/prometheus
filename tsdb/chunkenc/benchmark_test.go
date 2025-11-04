package chunkenc

import (
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
	export bench=chunkenc && go test \
	  -run '^$' -bench '^BenchmarkChunk' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkChunk(b *testing.B) {
	r := rand.New(rand.NewSource(1))

	for _, data := range []struct {
		name   string
		deltas func() (int64, float64)
	}{
		{
			name: "constant",
			deltas: func() (int64, float64) {
				return 1000, 0
			},
		},
		{
			name: "random steps",
			deltas: func() (int64, float64) {
				return int64(r.Intn(100) - 50 + 15000), // 15 seconds +- up to 100ms of jitter.
					float64(r.Intn(100) - 50) // Varying from -50 to +50 in 100 discrete steps.
			},
		},
		{
			name: "random 0-1",
			deltas: func() (int64, float64) {
				return int64(r.Intn(100) - 50 + 15000), // 15 seconds +- up to 100ms of jitter.
					r.Float64() // Random between 0 and 1.0.
			},
		},
	} {
		b.Run("data="+data.name, func(b *testing.B) {
			b.Run("format=XOR", func(b *testing.B) {
				benchmarkChunk(b, data.deltas, func() Chunk { return NewXORChunk() })
			})
		})
	}
}

func benchmarkChunk(b *testing.B, deltas func() (int64, float64), newChunk func() Chunk) {
	var (
		t = int64(1234123324)
		v = 1243535.123
	)
	const nSamples = 120 // Same as tsdb.DefaultSamplesPerChunk.
	var samples []pair
	for range nSamples {
		dt, dv := deltas()
		t += dt
		v += dv
		samples = append(samples, pair{t: t, v: v})
	}

	// Measure encoding efficiency.
	b.Run("op=Append", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			c := newChunk()

			a, err := c.Appender()
			if err != nil {
				b.Fatalf("get appender: %s", err)
			}
			for _, p := range samples {
				a.Append(p.t, p.v)
			}
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")
		}
	})

	// Create a chunk for decoding and compact benchmarks.
	c := newChunk()
	a, err := c.Appender()
	if err != nil {
		b.Fatalf("get appender: %s", err)
	}
	for _, p := range samples {
		a.Append(p.t, p.v)
	}

	// Measure decoding efficiency.
	b.Run("op=Iterate", func(b *testing.B) {
		b.ReportAllocs()

		var (
			sink float64
			it   Iterator
		)
		for i := 0; b.Loop(); {
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")

			it := c.Iterator(it)
			for it.Next() == ValFloat {
				_, v := it.At()
				sink = v
				i++
			}
			if err := it.Err(); err != nil && !errors.Is(err, io.EOF) {
				require.NoError(b, err)
			}
			_ = sink
		}
	})

	// Measure size before and after compaction and it's efficiency.
	b.Run("op=Compact", func(b *testing.B) {
		for b.Loop() {
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")
			c.Compact()
			b.ReportMetric(float64(len(c.Bytes())), "B/compacted_chunk")
		}
	})
}
