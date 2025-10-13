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
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

type mockIndexWriter struct {
	seriesChunks []series
}

func copyChunk(c chunkenc.Chunk) (chunkenc.Chunk, error) {
	b := c.Bytes()
	nb := make([]byte, len(b))
	copy(nb, b)
	return chunkenc.FromData(c.Encoding(), nb)
}

func (mockIndexWriter) AddSymbol(string) error { return nil }
func (m *mockIndexWriter) AddSeries(_ storage.SeriesRef, l labels.Labels, chks ...chunks.Meta) error {
	// Copy chunks as their bytes are pooled.
	chksNew := make([]chunks.Meta, len(chks))
	for i, chk := range chks {
		c, err := copyChunk(chk.Chunk)
		if err != nil {
			return fmt.Errorf("mockIndexWriter: copy chunk: %w", err)
		}
		chksNew[i] = chunks.Meta{MaxTime: chk.MaxTime, MinTime: chk.MinTime, Chunk: c}
	}

	// We don't combine multiple same series together, by design as `AddSeries` requires full series to be saved.
	m.seriesChunks = append(m.seriesChunks, series{l: l, chunks: chksNew})
	return nil
}

func (mockIndexWriter) WriteLabelIndex([]string, []string) error { return nil }
func (mockIndexWriter) Close() error                             { return nil }

type mockBReader struct {
	ir   IndexReader
	cr   ChunkReader
	mint int64
	maxt int64
}

func (r *mockBReader) Index() (IndexReader, error)  { return r.ir, nil }
func (r *mockBReader) Chunks() (ChunkReader, error) { return r.cr, nil }
func (*mockBReader) Tombstones() (tombstones.Reader, error) {
	return tombstones.NewMemTombstones(), nil
}
func (r *mockBReader) Meta() BlockMeta { return BlockMeta{MinTime: r.mint, MaxTime: r.maxt} }
func (*mockBReader) Size() int64       { return 0 }
