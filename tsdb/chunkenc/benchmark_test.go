package chunkenc

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

type sampleCase struct {
	name    string
	samples []pair
}

type fmtCase struct {
	name       string
	newChunkFn func() Chunk
}

func sampleCases() []sampleCase {
	const nSamples = 120 // Same as tsdb.DefaultSamplesPerChunk.

	var (
		r     = rand.New(rand.NewSource(1))
		initT = int64(1234123324)
		initV = 1243535.123
	)

	return []sampleCase{
		{
			name: "constant",
			samples: func() (ret []pair) {
				t, v := initT, initV
				for range nSamples {
					t += 1000
					ret = append(ret, pair{t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "random steps",
			samples: func() (ret []pair) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += float64(r.Intn(100) - 50)       // Varying from -50 to +50 in 100 discrete steps.
					ret = append(ret, pair{t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "random 0-1",
			samples: func() (ret []pair) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					ret = append(ret, pair{t: t, v: v})
				}
				return ret
			}(),
		},
	}
}

func fmtCases() []fmtCase {
	return []fmtCase{
		{name: "XOR", newChunkFn: func() Chunk { return NewXORChunk() }},
	}
}

/*
	export bench=append && go test \
	  -run '^$' -bench '^BenchmarkAppender' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkAppender(b *testing.B) {
	for _, fc := range fmtCases() {
		for _, sc := range sampleCases() {
			b.Run(fmt.Sprintf("fmt=%s/samples=%s", fc.name, sc.name), func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					c := fc.newChunkFn()

					a, err := c.Appender()
					if err != nil {
						b.Fatalf("get appender: %s", err)
					}
					for _, p := range sc.samples {
						a.Append(p.t, p.v)
					}
					b.ReportMetric(float64(len(c.Bytes())), "B/chunk")
				}
			})
		}
	}
}

/*
	export bench=iter && go test \
	  -run '^$' -bench '^BenchmarkIterator' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkIterator(b *testing.B) {
	for _, fc := range fmtCases() {
		for _, sc := range sampleCases() {
			b.Run(fmt.Sprintf("fmt=%s/samples=%s", fc.name, sc.name), func(b *testing.B) {
				b.ReportAllocs()

				c := fc.newChunkFn()
				a, err := c.Appender()
				if err != nil {
					b.Fatalf("get appender: %s", err)
				}
				for _, p := range sc.samples {
					a.Append(p.t, p.v)
				}

				var (
					sink float64
					it   Iterator
				)
				// Measure decoding efficiency.
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
		}
	}
}

/*
	export bench=compact && go test \
	  -run '^$' -bench '^BenchmarkCompact' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkCompact(b *testing.B) {
	for _, fc := range fmtCases() {
		for _, sc := range sampleCases() {
			b.Run(fmt.Sprintf("fmt=%s/samples=%s", fc.name, sc.name), func(b *testing.B) {
				b.ReportAllocs()

				c := fc.newChunkFn()
				a, err := c.Appender()
				if err != nil {
					b.Fatalf("get appender: %s", err)
				}
				for _, p := range sc.samples {
					a.Append(p.t, p.v)
				}

				// Measure size before and after compaction and it's efficiency.
				for b.Loop() {
					b.ReportMetric(float64(len(c.Bytes())), "B/chunk")
					c.Compact()
					b.ReportMetric(float64(len(c.Bytes())), "B/compacted_chunk")
				}
			})
		}
	}
}
