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
		r      = rand.New(rand.NewSource(1))
		initST = timestamp.FromTime(d) // Use realistic timestamp.
		initT  = initST + 15000        // 15s after initST.
		initV  = 1243535.123
	)

	sampleCases := []sampleCase{
		{
			name: "vt=constant/st=0",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += 15000
					ret = append(ret, triple{st: 0, t: t, v: v})
				}
				return ret
			}(),
		},

		{
			// Cumulative with a constant ST through the whole chunk, typical case (e.g. long counting counter).
			name: "vt=constant/st=cumulative",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += 15000
					ret = append(ret, triple{st: initST, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			// Delta simulates delta type or worst case for cumulatives, where ST
			// is changing on every sample.
			name: "vt=constant/st=delta",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					st := t + 1 // ST is a tight interval after the last t+1ms.
					t += 15000
					ret = append(ret, triple{st: st, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random steps/st=0",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += float64(r.Intn(100) - 50)       // Varying from -50 to +50 in 100 discrete steps.
					ret = append(ret, triple{st: 0, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random steps/st=cumulative",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += float64(r.Intn(100) - 50)       // Varying from -50 to +50 in 100 discrete steps.
					ret = append(ret, triple{st: initST, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random steps/st=delta",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					st := t + 1                          // ST is a tight interval after the last t+1ms.
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += float64(r.Intn(100) - 50)       // Varying from -50 to +50 in 100 discrete steps.
					ret = append(ret, triple{st: st, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random 0-1/st=0",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					ret = append(ret, triple{st: 0, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			// Are we impacted by https://victoriametrics.com/blog/go-protobuf/ negative varint issue? (zig-zag needed?)
			name: "vt=negrandom 0-1/st=0",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v -= r.Float64()                     // Random between 0 and 1.0.
					ret = append(ret, triple{st: 0, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random 0-1/st=cumulative",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					ret = append(ret, triple{st: initST, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random 0-1/st=cumulative-periodic-resets",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for i := range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					st := initST
					if i%6 == 5 {
						st = t - 10000 // Reset of 10s before current t.
					}
					ret = append(ret, triple{st: st, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random 0-1/st=cumulative-periodic-zeros",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for i := range nSamples {
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					st := initST
					if i%6 == 5 {
						st = 0
					}
					ret = append(ret, triple{st: st, t: t, v: v})
				}
				return ret
			}(),
		},
		{
			name: "vt=random 0-1/st=delta",
			samples: func() (ret []triple) {
				t, v := initT, initV
				for range nSamples {
					st := t + 1                          // ST is a tight interval after the last t+1ms.
					t += int64(r.Intn(100) - 50 + 15000) // 15 seconds +- up to 100ms of jitter.
					v += r.Float64()                     // Random between 0 and 1.0.
					ret = append(ret, triple{st: st, t: t, v: v})
				}
				return ret
			}(),
		},
	}

	for _, f := range []fmtCase{
		{name: "XOR", newChunkFn: func() Chunk { return NewXORChunk() }, stUnsupported: true},
		{name: "XOR_OPT_ST", newChunkFn: func() Chunk { return NewXOROptSTChunk() }},
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
