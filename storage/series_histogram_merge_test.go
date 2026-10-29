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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func encoderMergeInput(tb testing.TB, c encoderCase, mode string, mergeFunc VerticalSeriesMergeFunc) ChunkSeries {
	tb.Helper()
	series, metas := encoderInput(tb, c)
	other := metas
	switch mode {
	case "different-cuts":
		samples, err := ExpandSamples(series.Iterator(nil), nil)
		require.NoError(tb, err)
		other = nil
		for start := 0; start < len(samples); start += 91 {
			end := min(start+91, len(samples))
			ms, err := ExpandChunks(NewSeriesToChunkEncoder(NewListSeries(labels.EmptyLabels(), samples[start:end])).Iterator(nil))
			require.NoError(tb, err)
			other = append(other, ms...)
		}
	case "disjoint":
		other = metas[len(metas)/2:]
		metas = metas[:len(metas)/2]
	}
	makeSeries := func(ms []chunks.Meta) ChunkSeries {
		return &ChunkSeriesEntry{ChunkIteratorFn: func(chunks.Iterator) chunks.Iterator { return NewListChunkSeriesIterator(ms...) }}
	}
	return NewCompactingChunkSeriesMerger(mergeFunc)(makeSeries(metas), makeSeries(other))
}

func TestCompactingChunkSeriesMergerHistogramTargets(t *testing.T) {
	for _, c := range encoderCases() {
		if c.list {
			continue
		}
		for _, mode := range []string{"identical", "different-cuts", "disjoint"} {
			t.Run(c.name+"/"+mode, func(t *testing.T) {
				nilTargets := func(series ...Series) Series {
					merged := ChainedSeriesMerge(series...)
					return &SeriesEntry{SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
						return nilHistogramTargetIterator{merged.Iterator(nil)}
					}}
				}
				want, err := ExpandChunks(encoderMergeInput(t, c, mode, nilTargets).Iterator(nil))
				require.NoError(t, err)
				got, err := ExpandChunks(encoderMergeInput(t, c, mode, ChainedSeriesMerge).Iterator(nil))
				require.NoError(t, err)
				require.Len(t, got, len(want))
				for i := range want {
					require.Equal(t, want[i].MinTime, got[i].MinTime)
					require.Equal(t, want[i].MaxTime, got[i].MaxTime)
					require.Equal(t, want[i].Chunk.Encoding(), got[i].Chunk.Encoding())
					require.Equal(t, want[i].Chunk.Bytes(), got[i].Chunk.Bytes())
				}
			})
		}
	}
}

func BenchmarkCompactingChunkSeriesMerger(b *testing.B) {
	// Identical and disjoint inputs are controls that bypass re-encoding.
	for _, c := range []encoderCase{
		{name: "integer8", kind: "integer", shape: "stable", buckets: 8},
		{name: "float8", kind: "float", shape: "stable", buckets: 8},
		{name: "float128", kind: "float", shape: "stable", buckets: 128},
		{name: "scalar", kind: "scalar"},
	} {
		for _, mode := range []string{"identical", "different-cuts", "disjoint"} {
			b.Run(c.name+"/"+mode, func(b *testing.B) {
				merged := encoderMergeInput(b, c, mode, ChainedSeriesMerge)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					it := merged.Iterator(nil)
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
}
