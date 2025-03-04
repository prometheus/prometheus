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
type IndexReader struct {
	ix Index
}

// ChunkReader implements the tsdb.ChunkReader interface.
type ChunkReader struct{}

// The index reader.

// NewIndexReader (dir string).
func NewIndexReader(dir string) (*IndexReader, error) {
	index, err := ReadIndex(dir)
	if err != nil {
		return nil, err
	}

	return &IndexReader{
		ix: index,
	}, nil
}

// Symbols returns an empty iterator since we don't build and need a symbol
// table at this point.
func (ir *IndexReader) Symbols() index.StringIter {
	return NopStringIter{}
}

// SymbolTableSize returns 0 since we don't build and need a symbol table at
// this point.
func (ir *IndexReader) SymbolTableSize() uint64 {
	return 0
}

// SortedLabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *IndexReader) SortedLabelValues(_ context.Context, _ string, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented: SortedLabelValues")
}

// LabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *IndexReader) LabelValues(_ context.Context, _ string, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented: LabelValues")
}

// Postings (ctx context.Context, name string, values ...string).
func (ir *IndexReader) Postings(_ context.Context, _ string, _ ...string) (index.Postings, error) {
	return nil, errors.New("not implemented: Postings")
}

// PostingsForLabelMatching (ctx context.Context, name string, match func(value string) bool).
func (ir *IndexReader) PostingsForLabelMatching(_ context.Context, _ string, _ func(value string) bool) index.Postings {
	panic("not implemented: PostingsForLabelMatching")
}

// PostingsForAllLabelValues (ctx context.Context, name string).
func (ir *IndexReader) PostingsForAllLabelValues(_ context.Context, _ string) index.Postings {
	panic("not implemented: PostingsForAllLabelValues")
}

// SortedPostings (p index.Postings).
func (ir *IndexReader) SortedPostings(_ index.Postings) index.Postings {
	panic("not implemented: SortedPostings")
}

// ShardedPostings (p index.Postings, shardIndex, shardCount uint64).
func (ir *IndexReader) ShardedPostings(_ index.Postings, _, _ uint64) index.Postings {
	panic("not implemented: ShardedPostings")
}

// Series (ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta).
func (ir *IndexReader) Series(_ storage.SeriesRef, _ *labels.ScratchBuilder, _ *[]chunks.Meta) error {
	panic("not implemented: Series")
}

// LabelNames (ctx context.Context, matchers ...*labels.Matcher).
func (ir *IndexReader) LabelNames(_ context.Context, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented: LabelNames")
}

// LabelValueFor (ctx context.Context, id storage.SeriesRef, label string).
func (ir *IndexReader) LabelValueFor(_ context.Context, _ storage.SeriesRef, _ string) (string, error) {
	return "", errors.New("not implemented: LabelValueFor")
}

// LabelNamesFor (ctx context.Context, postings index.Postings).
func (ir *IndexReader) LabelNamesFor(_ context.Context, _ index.Postings) ([]string, error) {
	return nil, errors.New("not implemented: LabelNamesFor")
}

func (ir *IndexReader) Size() int64 {
	// TODO: implement this method.
	return 0
}

// Close ().
func (ir *IndexReader) Close() error {
	// Do nothing.
	return nil
}

// The chunks reader.

// NewChunkReader (dir string).
func NewChunkReader(_ string) (*ChunkReader, error) {
	return &ChunkReader{}, nil
}

// ChunkOrIterable (meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error).
func (cr *ChunkReader) ChunkOrIterable(_ chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	return nil, nil, errors.New("not implemented: ChunkOrIterable")
}

func (cr *ChunkReader) Size() int64 {
	// TODO: implement this method.
	return 0
}

// Close ().
func (cr *ChunkReader) Close() error {
	// Do nothing.
	return nil
}

// NopStringIter implements tsdb.StringIter.
type NopStringIter struct{}

func (it NopStringIter) Next() bool {
	return false
}

func (it NopStringIter) At() string {
	return ""
}

func (it NopStringIter) Err() error {
	return nil
}

var _ index.StringIter = NopStringIter{}
