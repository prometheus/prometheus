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

package storage

import (
	"fmt"
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/stretchr/testify/require"
)

type encoderCase struct {
	name, kind, shape string
	buckets           int
	st, list          bool
}

func encoderCases() []encoderCase {
	var cases []encoderCase
	for _, kind := range []string{"integer", "float"} {
		for _, n := range []int{8, 128} {
			for _, shape := range []string{"stable", "grow", "stale", "schema", "gauge", "reset", "custom", "negative", "zero"} {
				cases = append(cases, encoderCase{name: fmt.Sprintf("%s/%s/%d", kind, shape, n), kind: kind, shape: shape, buckets: n})
			}
		}
		cases = append(cases, encoderCase{name: kind + "/ST", kind: kind, shape: "stale", buckets: 8, st: true}, encoderCase{name: kind + "/list", kind: kind, shape: "stable", buckets: 8, list: true})
	}
	cases = append(cases, encoderCase{name: "scalar/XOR", kind: "scalar"}, encoderCase{name: "scalar/XOR2", kind: "scalar", st: true}, encoderCase{name: "mixed", kind: "mixed", shape: "stale", buckets: 8})
	return cases
}

func encoderInput(tb testing.TB, c encoderCase) (Series, []chunks.Meta) {
	tb.Helper()
	const count = 240
	in := make([]chunks.Sample, 0, count)
	for i := range count {
		st := int64(0)
		if c.st {
			st = 1
		}
		ts := int64(i+1) * 15000
		kind := c.kind
		if kind == "mixed" {
			kind = []string{"integer", "float", "scalar"}[(i/40)%3]
		}
		if kind == "scalar" {
			in = append(in, fSample{st, ts, float64(i)})
			continue
		}
		n := c.buckets
		if c.shape == "grow" && i%73 >= 36 {
			n += 2
		}
		step := i + 1
		if c.shape == "reset" {
			step = i%57 + 1
		}
		h := &histogram.Histogram{Schema: 0, Count: uint64(step * n), Sum: float64(step * n), PositiveSpans: []histogram.Span{{Offset: 0, Length: uint32(n)}}, PositiveBuckets: make([]int64, n)}
		h.PositiveBuckets[0] = int64(step)
		if c.shape == "negative" {
			h.NegativeSpans = []histogram.Span{{Offset: 0, Length: uint32(n)}}
			h.NegativeBuckets = append([]int64(nil), h.PositiveBuckets...)
			h.Count *= 2
		}
		if c.shape == "zero" {
			h.ZeroThreshold = 0.001
			h.ZeroCount = uint64(step)
			h.Count += h.ZeroCount
		}
		if c.shape == "schema" {
			h.Schema = int32((i / 37) % 3)
		}
		if c.shape == "gauge" {
			h.CounterResetHint = histogram.GaugeType
		}
		if c.shape == "custom" {
			h.Schema = histogram.CustomBucketsSchema
			h.CustomValues = make([]float64, n-1)
			for j := range h.CustomValues {
				h.CustomValues[j] = float64(j + 1)
			}
		}
		if c.shape == "stale" && i%79 == 40 {
			h = &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}
		}
		if kind == "integer" {
			in = append(in, hSample{st, ts, h})
		} else {
			in = append(in, fhSample{st, ts, h.ToFloat(nil)})
		}
	}
	if c.list {
		return NewListSeries(labels.EmptyLabels(), in), nil
	}
	var metas []chunks.Meta
	// Input chunk boundaries differ from the encoder's 120-sample split.
	for start := 0; start < len(in); start += 73 {
		end := min(start+73, len(in))
		ms, err := ExpandChunks(NewSeriesToChunkEncoder(NewListSeries(labels.EmptyLabels(), in[start:end])).Iterator(nil))
		require.NoError(tb, err)
		metas = append(metas, ms...)
	}
	ss := make([]Series, 0, len(metas))
	for _, m := range metas {
		ss = append(ss, &SeriesEntry{SampleIteratorFn: func(it chunkenc.Iterator) chunkenc.Iterator { return m.Chunk.Iterator(it) }})
	}
	return ChainedSeriesMerge(ss...), metas
}

func BenchmarkSeriesToChunkEncoder(b *testing.B) {
	for _, c := range encoderCases() {
		b.Run(c.name, func(b *testing.B) {
			series, _ := encoderInput(b, c)
			encoder := NewSeriesToChunkEncoder(series)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				it := encoder.Iterator(nil)
				n := 0
				for it.Next() {
					n += it.At().Chunk.NumSamples()
				}
				if it.Err() != nil || n != 240 {
					b.Fatalf("samples=%d err=%v", n, it.Err())
				}
			}
		})
	}
}

// nilHistogramTargetIterator exercises the original decoding ownership path
// regardless of which target the encoder supplies.
type nilHistogramTargetIterator struct {
	chunkenc.Iterator
}

func (it nilHistogramTargetIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return it.Iterator.AtHistogram(nil)
}

func (it nilHistogramTargetIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return it.Iterator.AtFloatHistogram(nil)
}

func TestSeriesToChunkEncoderHistogramTargets(t *testing.T) {
	for _, c := range encoderCases() {
		t.Run(c.name, func(t *testing.T) {
			reference, _ := encoderInput(t, c)
			nilTargets := &SeriesEntry{SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
				return nilHistogramTargetIterator{reference.Iterator(nil)}
			}}
			want, err := ExpandChunks(NewSeriesToChunkEncoder(nilTargets).Iterator(nil))
			require.NoError(t, err)

			series, input := encoderInput(t, c)
			before := make([][]byte, len(input))
			for i, meta := range input {
				before[i] = append([]byte(nil), meta.Chunk.Bytes()...)
			}
			encoder := NewSeriesToChunkEncoder(series)
			var it chunks.Iterator
			for range 3 {
				it = encoder.Iterator(it)
				got, err := ExpandChunks(it)
				require.NoError(t, err)
				require.Len(t, got, len(want))
				for i := range want {
					require.Equal(t, want[i].MinTime, got[i].MinTime)
					require.Equal(t, want[i].MaxTime, got[i].MaxTime)
					require.Equal(t, want[i].Chunk.Encoding(), got[i].Chunk.Encoding())
					require.Equal(t, want[i].Chunk.Bytes(), got[i].Chunk.Bytes())
				}
			}
			for i, meta := range input {
				require.Equal(t, before[i], meta.Chunk.Bytes())
			}
		})
	}
}
