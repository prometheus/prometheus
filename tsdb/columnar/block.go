// Copyright 2025 The Prometheus Authors

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

package columnar

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

// IndexReader implements the tsdb.IndexReader interface.
type IndexReader struct{}

// ChunkReader implements the tsdb.ChunkReader interface.
type ChunkReader struct{}

// The index reader.

// NewIndexReader (dir string).
func NewIndexReader(_ string) (*IndexReader, error) {
	return &IndexReader{}, nil
}

func (ir *IndexReader) Symbols() index.StringIter {
	panic("not implemented")
}

func (ir *IndexReader) SymbolTableSize() uint64 {
	// TODO: implement this method.
	return 0
}

// SortedLabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *IndexReader) SortedLabelValues(_ context.Context, _ string, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

// LabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *IndexReader) LabelValues(_ context.Context, _ string, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

// Postings (ctx context.Context, name string, values ...string).
func (ir *IndexReader) Postings(_ context.Context, _ string, _ ...string) (index.Postings, error) {
	return nil, errors.New("not implemented")
}

// PostingsForLabelMatching (ctx context.Context, name string, match func(value string) bool).
func (ir *IndexReader) PostingsForLabelMatching(_ context.Context, _ string, _ func(value string) bool) index.Postings {
	panic("not implemented")
}

// PostingsForAllLabelValues (ctx context.Context, name string).
func (ir *IndexReader) PostingsForAllLabelValues(_ context.Context, _ string) index.Postings {
	panic("not implemented")
}

// SortedPostings (p index.Postings).
func (ir *IndexReader) SortedPostings(_ index.Postings) index.Postings {
	panic("not implemented")
}

// ShardedPostings (p index.Postings, shardIndex, shardCount uint64).
func (ir *IndexReader) ShardedPostings(_ index.Postings, _, _ uint64) index.Postings {
	panic("not implemented")
}

// Series (ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta).
func (ir *IndexReader) Series(_ storage.SeriesRef, _ *labels.ScratchBuilder, _ *[]chunks.Meta) error {
	panic("not implemented")
}

// LabelNames (ctx context.Context, matchers ...*labels.Matcher).
func (ir *IndexReader) LabelNames(_ context.Context, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

// LabelValueFor (ctx context.Context, id storage.SeriesRef, label string).
func (ir *IndexReader) LabelValueFor(_ context.Context, _ storage.SeriesRef, _ string) (string, error) {
	return "", errors.New("not implemented")
}

// LabelNamesFor (ctx context.Context, postings index.Postings).
func (ir *IndexReader) LabelNamesFor(_ context.Context, _ index.Postings) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *IndexReader) Size() int64 {
	// TODO: implement this method.
	return 0
}

// Close ().
func (ir *IndexReader) Close() error {
	return errors.New("not implemented")
}

// The chunks reader.

// NewChunkReader (dir string).
func NewChunkReader(_ string) (*ChunkReader, error) {
	return &ChunkReader{}, nil
}

// ChunkOrIterable (meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error).
func (cr *ChunkReader) ChunkOrIterable(_ chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	return nil, nil, errors.New("not implemented")
}

func (cr *ChunkReader) Size() int64 {
	// TODO: implement this method.
	return 0
}

// Close ().
func (cr *ChunkReader) Close() error {
	return errors.New("not implemented")
}
