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
	"io"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/timestamp"
)

type sampleCase struct {
	name    string
	samples []triple
}

type fmtCase struct {
	name          string
	newChunkFn    func() Chunk
	stUnsupported bool
}

func foreachFmtSampleCase(b *testing.B, fn func(b *testing.B, f fmtCase, s sampleCase)) {
	const nSamples = 120 // Same as tsdb.DefaultSamplesPerChunk.

	d, err := time.Parse(time.DateTime, "2025-11-04 10:01:05")
	require.NoError(b, err)

	var (
		r       = rand.New(rand.NewSource(1)) // Fixed seed for reproducibility.
		initST  = timestamp.FromTime(d)       // Use realistic timestamp.
		initT   = initST + 15000              // 15s after initST.
		initV   = 1243535.123
		rInts   = make([]int64, 2*nSamples) // Random ints for timestamps and STs.
		rFloats = make([]float64, nSamples)
	)

	// Pre-generate random numbers so that adding/removing cases does not change
	// the generated samples.
	for i := range nSamples {
		rInts[i] = int64(r.Intn(100))
		rInts[nSamples+i] = int64(r.Intn(100))
		rFloats[i] = float64(r.Intn(100))
	}

	// tPatterns control how the regular timestamp advances.
	type tPattern struct {
		name string
		next func(t int64, i int) int64
	}
	// vPatterns control how the value advances.
	type vPattern struct {
		name string
		next func(v float64, i int) float64
	}
	// stPatterns compute the start timestamp from the previous t (before the
	// step), the new t (after the step), and the sample index.
	type stPattern struct {
		name    string
		compute func(prevT, newT int64, i int) int64
	}

	tPatterns := []tPattern{
		{
			name: "t=constant",
			next: func(t int64, _ int) int64 { return t + 15000 },
		},
		{
			// 15 seconds ± up to 100ms of jitter.
			name: "t=jitter",
			next: func(t int64, i int) int64 { return t + rInts[i] - 50 + 15000 },
		},
		{
			// First 10 samples at constant 60s, then one 10-interval gap (600s),
			// then 60s ± 30ms jitter. The gap triggers XOR18111 full mode via
			// multiplier encoding (dod=540000 = 9×60000). Subsequent small-jitter
			// delta-of-deltas (≤30ms) use XOR18111's 7-bit full-mode code (9 bits
			// total) vs XOR compact's minimum 14-bit code (16 bits total).
			name: "t=gap-jitter",
			next: func(t int64, i int) int64 {
				if i < 10 {
					return t + 60000
				}
				if i == 10 {
					return t + 10*60000 // 10-interval gap; triggers XOR18111 full mode.
				}
				return t + 60000 + rInts[i]%61 - 30 // 60s ± 30ms jitter.
			},
		},
	}
	vPatterns := []vPattern{
		{
			name: "v=constant",
			next: func(v float64, _ int) float64 { return v },
		},
		// We are not interested in float compression we're not changing it.
		// {
		// 	// Varying from -50 to +50 in 100 discrete steps.
		// 	name: "v=rand-steps",
		// 	next: func(v float64, i int) float64 { return v + rFloats[i] - 50 },
		// },
		// {
		// 	// Random increment between 0 and 1.0.
		// 	name: "v=rand0-1",
		// 	next: func(v float64, i int) float64 { return v + rFloats[i]/100.0 },
		// },
		// {
		// 	// Random decrement between 0 and -1.0. Tests negative varint encoding;
		// 	// see https://victoriametrics.com/blog/go-protobuf/.
		// 	name: "v=nrand0-1",
		// 	next: func(v float64, i int) float64 { return v - rFloats[i]/100.0 },
		// },
	}
	stPatterns := []stPattern{
		{
			name:    "st=0",
			compute: func(_, _ int64, _ int) int64 { return 0 },
		},
		{
			// Constant ST throughout the chunk, typical for long-running counters.
			name:    "st=cumulative",
			compute: func(_, _ int64, _ int) int64 { return initST },
		},
		{
			// ST is just after the previous sample's t: tight delta interval.
			name:    "st=delta-excl",
			compute: func(prevT, _ int64, _ int) int64 { return prevT + 1 },
		},
		{
			// ST equals the previous sample's t: inclusive delta interval.
			name:    "st=delta-incl",
			compute: func(prevT, _ int64, _ int) int64 { return prevT },
		},
		{
			// ST equals the current sample's t.
			name:    "st=t",
			compute: func(_, newT int64, _ int) int64 { return newT },
		},
		{
			// ST is equal to the previous t plus up to 100ms of jitter.
			name:    "st=delta-jitter",
			compute: func(prevT, _ int64, i int) int64 { return prevT + rInts[nSamples+i] },
		},
		{
			// Cumulative ST with periodic resets 10s before the current t.
			name: "st=cum-resets",
			compute: func(_, newT int64, i int) int64 {
				if i%6 == 5 {
					return newT - 10000
				}
				return initST
			},
		},
		{
			// Cumulative ST with periodic zero resets.
			name: "st=cum-zeros",
			compute: func(_, _ int64, i int) int64 {
				if i%6 == 5 {
					return 0
				}
				return initST
			},
		},
	}

	var sampleCases []sampleCase
	for _, tp := range tPatterns {
		for _, vp := range vPatterns {
			for _, sp := range stPatterns {
				samples := make([]triple, 0, nSamples)
				t, v := initT, initV
				for i := range nSamples {
					prevT := t
					t = tp.next(t, i)
					v = vp.next(v, i)
					st := sp.compute(prevT, t, i)
					samples = append(samples, triple{st: st, t: t, v: v})
				}
				sampleCases = append(sampleCases, sampleCase{
					name:    tp.name + "/" + vp.name + "/" + sp.name,
					samples: samples,
				})
			}
		}
	}

	for _, f := range []fmtCase{
		{name: "XOR", newChunkFn: func() Chunk { return NewXORChunk() }, stUnsupported: true},
		{name: "XOR2", newChunkFn: func() Chunk { return NewXOR2Chunk() }, stUnsupported: true},
		{name: "XOR2ST", newChunkFn: func() Chunk { return NewXOR2STChunk() }},
		{name: "XOR2ST_OTEL", newChunkFn: func() Chunk { return NewXOR2STotelChunk() }},
		{name: "XOR_OPT_ST", newChunkFn: func() Chunk { return NewXOROptSTChunk() }},
		{name: "XOR_OPT_ST_OTEL", newChunkFn: func() Chunk { return NewXOROptSTotelChunk() }},
		{name: "XOR18111", newChunkFn: func() Chunk { return NewXOR18111Chunk() }, stUnsupported: true},
		{name: "XOR18111ST", newChunkFn: func() Chunk { return NewXOR18111STChunk() }},
		{name: "XOR18111ST2", newChunkFn: func() Chunk { return NewXOR18111ST2Chunk() }},
	} {
		for _, s := range sampleCases {
			b.Run(fmt.Sprintf("fmt=%s/%s", f.name, s.name), func(b *testing.B) {
				fn(b, f, s)
			})
		}
	}
}

/*
	export bench=bw.bench/append.v2 && go test \
	  -run '^$' -bench '^BenchmarkAppender' \
	  -benchtime 1s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt

For profiles:

	 export bench=bw.bench/appendprof && go test \
		  -run '^$' -bench '^BenchmarkAppender' \
		  -benchtime 1s -count 1 -cpu 2 -timeout 999m \
		  -cpuprofile=${bench}.cpu.pprof \
		  | tee ${bench}.txt
*/
func BenchmarkAppender(b *testing.B) {
	foreachFmtSampleCase(b, func(b *testing.B, f fmtCase, s sampleCase) {
		b.ReportAllocs()

		for b.Loop() {
			c := f.newChunkFn()

			a, err := c.Appender()
			if err != nil {
				b.Fatalf("get appender: %s", err)
			}
			for _, p := range s.samples {
				a.Append(p.st, p.t, p.v)
			}
			// NOTE: Some buffered implementations only encode on Bytes().
			b.ReportMetric(float64(len(c.Bytes())), "B/chunk")

			require.Equal(b, len(s.samples), c.NumSamples())
		}
	})
}

/*
	export bench=bw.bench/iter && go test \
	  -run '^$' -bench '^BenchmarkIterator' \
	  -benchtime 1s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt

For profiles:

	 export bench=bw.bench/iterprof && go test \
		  -run '^$' -bench '^BenchmarkIterator' \
		  -benchtime 1000000x -count 1 -cpu 2 -timeout 999m \
	  -cpuprofile=${bench}.cpu.pprof \
		  | tee ${bench}.txt
	 export bench=bw.bench/iterprof && go test \
		  -run '^$' -bench '^BenchmarkIterator' \
		  -benchtime 1000000x -count 1 -cpu 2 -timeout 999m \
	  -memprofile=${bench}.mem.pprof \
		  | tee ${bench}.txt
*/
func BenchmarkIterator(b *testing.B) {
	foreachFmtSampleCase(b, func(b *testing.B, f fmtCase, s sampleCase) {
		floatEquals := func(a, b float64) bool {
			return a == b
		}
		if f.name == "ALPBuffered" {
			// Hack as ALP loses precision.
			floatEquals = func(a, b float64) bool {
				return math.Abs(a-b) < 1e-6
			}
		}
		b.ReportAllocs()

		c := f.newChunkFn()
		a, err := c.Appender()
		if err != nil {
			b.Fatalf("get appender: %s", err)
		}
		for _, p := range s.samples {
			a.Append(p.st, p.t, p.v)
		}

		// Some chunk implementations might be buffered. Reset to ensure we don't reuse
		// appending buffers.
		c.Reset(c.Bytes())

		// While we are at it, test if encoding/decoding works.
		it := c.Iterator(nil)
		require.Equal(b, len(s.samples), c.NumSamples())
		var got []triple
		for i := 0; it.Next() == ValFloat; i++ {
			t, v := it.At()
			got = append(got, triple{st: it.AtST(), t: t, v: v})
		}
		if err := it.Err(); err != nil && !errors.Is(err, io.EOF) {
			require.NoError(b, err)
		}
		expectedSamples := s.samples
		if f.stUnsupported {
			// If the format does not support ST, zero them out for comparison.
			expectedSamples = make([]triple, len(s.samples))
			copy(expectedSamples, s.samples)
			for i := range s.samples {
				expectedSamples[i].st = 0
			}
		}
		if diff := cmp.Diff(expectedSamples, got, cmp.AllowUnexported(triple{}), cmp.Comparer(floatEquals)); diff != "" {
			b.Fatalf("mismatch (-want +got):\n%s", diff)
		}

		var sink float64
		// Measure decoding efficiency.
		for i := 0; b.Loop(); {
			// Some chunk implementations might be buffered. Reset to ensure we don't reuse
			// previous decoded data.
			c.Reset(c.Bytes())
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
