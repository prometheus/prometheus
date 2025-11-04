package chunkenc

import (
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

/*
	export bench=chunkenc && go test \
	  -run '^$' -bench '^BenchmarkChunkEnc' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkChunkEnc(b *testing.B) {
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
				benchmarkChunkEnc(b, data.deltas, func() Chunk { return NewXORChunk() })
			})
		})
	}
}

func benchmarkChunkEnc(b *testing.B, deltas func() (int64, float64), newChunk func() Chunk) {
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

/*
	export bench=histchunkenc && go test \
	  -run '^$' -bench '^BenchmarkHistogramChunkEnc' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkHistogramChunkEnc(b *testing.B) {
	// Create a histogram with a bunch of spans and buckets.
	const (
		numSpans   = 1000
		spanLength = 10
	)
	h := &histogram.Histogram{
		Schema:        0,
		Count:         100,
		Sum:           1000,
		ZeroThreshold: 0.001,
		ZeroCount:     5,
	}
	for range numSpans {
		h.PositiveSpans = append(h.PositiveSpans, histogram.Span{Offset: 5, Length: spanLength})
		h.NegativeSpans = append(h.NegativeSpans, histogram.Span{Offset: 5, Length: spanLength})
		for j := range spanLength {
			h.PositiveBuckets = append(h.PositiveBuckets, int64(j))
			h.NegativeBuckets = append(h.NegativeBuckets, int64(j))
		}
	}

	// Measure encoding efficiency.
	b.Run("op=Append", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			c := Chunk(NewHistogramChunk())

			a, err := c.Appender()
			if err != nil {
				b.Fatalf("get appender: %s", err)
			}
			var stop bool
			for i := int64(0); !stop; i++ {
				_, stop, _, err = a.AppendHistogram(nil, i, h, true)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")
		}
	})

	// Create a chunk for decoding and compact benchmarks.
	c := Chunk(NewHistogramChunk())
	a, err := c.Appender()
	if err != nil {
		b.Fatalf("get appender: %s", err)
	}
	var stop bool
	for i := int64(0); !stop; i++ {
		_, stop, _, err = a.AppendHistogram(nil, i, h, true)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Measure decoding efficiency.
	b.Run("op=Iterate", func(b *testing.B) {
		b.ReportAllocs()

		var (
			sink *histogram.Histogram
			it   Iterator
		)
		for i := 0; b.Loop(); {
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")

			it := c.Iterator(it)
			for it.Next() == ValFloat {
				_, got := it.AtHistogram(h)
				sink = got
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
