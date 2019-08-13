// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/labels"
)

type mockIndexWriter struct {
	series []seriesSamples
}

func (mockIndexWriter) AddSymbols(sym map[string]struct{}) error { return nil }
func (m *mockIndexWriter) AddSeries(ref uint64, l labels.Labels, chunks ...chunks.Meta) error {
	i := -1
	for j, s := range m.series {
		if !labels.FromMap(s.lset).Equals(l) {
			continue
		}
		i = j
		break
	}
	if i == -1 {
		m.series = append(m.series, seriesSamples{
			lset: l.Map(),
		})
		i = len(m.series) - 1
	}

	var iter chunkenc.Iterator
	for _, chk := range chunks {
		samples := make([]sample, 0, chk.Chunk.NumSamples())

		iter = chk.Chunk.Iterator(iter)
		for iter.Next() {
			s := sample{}
			s.t, s.v = iter.At()

			samples = append(samples, s)
		}
		if err := iter.Err(); err != nil {
			return err
		}

		m.series[i].chunks = append(m.series[i].chunks, samples)
	}
	return nil
}

func (mockIndexWriter) WriteLabelIndex(names []string, values []string) error     { return nil }
func (mockIndexWriter) WritePostings(name, value string, it index.Postings) error { return nil }
func (mockIndexWriter) Close() error                                              { return nil }

type mockBReader struct {
	ir   IndexReader
	cr   ChunkReader
	mint int64
	maxt int64
}

func (r *mockBReader) Index() (IndexReader, error)          { return r.ir, nil }
func (r *mockBReader) Chunks() (ChunkReader, error)         { return r.cr, nil }
func (r *mockBReader) Tombstones() (TombstoneReader, error) { return newMemTombstones(), nil }
func (r *mockBReader) Meta() BlockMeta                      { return BlockMeta{MinTime: r.mint, MaxTime: r.maxt} }
