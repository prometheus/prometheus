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
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestChainSampleIteratorInitialState(t *testing.T) {
	constructors := map[string]func(chunkenc.Iterator, []Series) chunkenc.Iterator{
		"series": ChainSampleIteratorFromSeries,
		"iterables": func(reuse chunkenc.Iterator, input []Series) chunkenc.Iterator {
			iterables := make([]chunkenc.Iterable, len(input))
			for i, s := range input {
				iterables[i] = s
			}
			return ChainSampleIteratorFromIterables(reuse, iterables)
		},
		"iterators": func(reuse chunkenc.Iterator, input []Series) chunkenc.Iterator {
			iterators := make([]chunkenc.Iterator, len(input))
			for i, s := range input {
				iterators[i] = s.Iterator(nil)
			}
			return ChainSampleIteratorFromIterators(reuse, iterators)
		},
	}
	for name, construct := range constructors {
		for _, tc := range []struct {
			name        string
			input       [][]int64
			expected    []int64
			reuse, seek bool
		}{
			{name: "minimum timestamp", input: [][]int64{{math.MinInt64, math.MinInt64 + 1}}, expected: []int64{math.MinInt64, math.MinInt64 + 1}},
			{name: "minimum overlapping timestamp", input: [][]int64{{math.MinInt64, 0}, {math.MinInt64, math.MinInt64 + 1}}, expected: []int64{math.MinInt64, math.MinInt64 + 1, 0}},
			{name: "initial minimum seek", input: [][]int64{{2}}, expected: []int64{2}, seek: true},
			{name: "reused single input", input: [][]int64{{2}}, expected: []int64{2}, reuse: true, seek: true},
			{name: "reused overlapping inputs", input: [][]int64{{3}, {2, 3}}, expected: []int64{2, 3}, reuse: true, seek: true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				var it chunkenc.Iterator
				if tc.reuse {
					it = construct(nil, []Series{NewListSeries(labels.EmptyLabels(), []chunks.Sample{fSample{t: 1, f: 1}})})
					require.Equal(t, chunkenc.ValFloat, it.Next())
					require.Equal(t, int64(1), it.AtT())
				}
				input := make([]Series, len(tc.input))
				for i, timestamps := range tc.input {
					values := make([]chunks.Sample, len(timestamps))
					for j, ts := range timestamps {
						values[j] = fSample{t: ts, f: float64(ts)}
					}
					input[i] = NewListSeries(labels.EmptyLabels(), values)
				}
				it = construct(it, input)
				for i, expected := range tc.expected {
					var typ chunkenc.ValueType
					if i == 0 && tc.seek {
						typ = it.Seek(math.MinInt64)
					} else {
						typ = it.Next()
					}
					require.Equal(t, chunkenc.ValFloat, typ)
					timestamp, value := it.At()
					require.Equal(t, expected, timestamp)
					require.Equal(t, float64(expected), value)
				}
				require.Equal(t, chunkenc.ValNone, it.Next())
				require.NoError(t, it.Err())
			})
		}
	}
}
