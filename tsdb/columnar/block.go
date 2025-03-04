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
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

// ColumnarIndexReader implements the tsdb.IndexReader interface.
type ColumnarIndexReader struct {
	ix Index
}

// The columnar index reader.

// NewColumnarIndexReader (dir string).
func NewColumnarIndexReader(dir string) (*ColumnarIndexReader, error) {
	index, err := ReadIndex(dir)
	if err != nil {
		return nil, err
	}

	return &ColumnarIndexReader{
		ix: index,
	}, nil
}

// Symbols returns an empty iterator since we don't build and need a symbol
// table at this point.
func (ir *ColumnarIndexReader) Symbols() index.StringIter {
	return NopStringIter{}
}

// SymbolTableSize returns 0 since we don't build and need a symbol table at
// this point.
func (ir *ColumnarIndexReader) SymbolTableSize() uint64 {
	return 0
}

// SortedLabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *ColumnarIndexReader) SortedLabelValues(_ context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	if name == labels.MetricName {
		if len(matchers) == 0 {
			nameValues := make([]string, 0, len(ir.ix.Metrics))
			for name := range ir.ix.Metrics {
				nameValues = append(nameValues, name)
			}
			// sort nameValues by string value
			sort.Strings(nameValues)
			return nameValues, nil
		}
	}

	return nil, errors.New("not implemented: SortedLabelValues")
}

// LabelValues (ctx context.Context, name string, matchers ...*labels.Matcher).
func (ir *ColumnarIndexReader) LabelValues(_ context.Context, _ string, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented: LabelValues")
}

// Postings (ctx context.Context, name string, values ...string).
func (ir *ColumnarIndexReader) Postings(_ context.Context, name string, values ...string) (index.Postings, error) {
	return nil, fmt.Errorf("not implemented: Postings name=%s values=%v", name, values)
}

// PostingsForLabelMatching (ctx context.Context, name string, match func(value string) bool).
func (ir *ColumnarIndexReader) PostingsForLabelMatching(_ context.Context, _ string, _ func(value string) bool) index.Postings {
	panic("not implemented: PostingsForLabelMatching")
}

// PostingsForAllLabelValues (ctx context.Context, name string).
func (ir *ColumnarIndexReader) PostingsForAllLabelValues(_ context.Context, _ string) index.Postings {
	panic("not implemented: PostingsForAllLabelValues")
}

// SortedPostings (p index.Postings).
func (ir *ColumnarIndexReader) SortedPostings(_ index.Postings) index.Postings {
	panic("not implemented: SortedPostings")
}

// ShardedPostings (p index.Postings, shardIndex, shardCount uint64).
func (ir *ColumnarIndexReader) ShardedPostings(_ index.Postings, _, _ uint64) index.Postings {
	panic("not implemented: ShardedPostings")
}

// Series (ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta).
func (ir *ColumnarIndexReader) Series(_ storage.SeriesRef, _ *labels.ScratchBuilder, _ *[]chunks.Meta) error {
	panic("not implemented: Series")
}

// LabelNames (ctx context.Context, matchers ...*labels.Matcher).
func (ir *ColumnarIndexReader) LabelNames(_ context.Context, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented: LabelNames")
}

// LabelValueFor (ctx context.Context, id storage.SeriesRef, label string).
func (ir *ColumnarIndexReader) LabelValueFor(_ context.Context, _ storage.SeriesRef, _ string) (string, error) {
	return "", errors.New("not implemented: LabelValueFor")
}

// LabelNamesFor (ctx context.Context, postings index.Postings).
func (ir *ColumnarIndexReader) LabelNamesFor(_ context.Context, _ index.Postings) ([]string, error) {
	return nil, errors.New("not implemented: LabelNamesFor")
}

func (ir *ColumnarIndexReader) Size() int64 {
	// TODO: implement this method.
	return 0
}

// Close ().
func (ir *ColumnarIndexReader) Close() error {
	// Do nothing.
	return nil
}

// The chunks reader.

// ColumnarChunkReader implements the tsdb.ChunkReader interface.
type ColumnarChunkReader struct {
	dir      string
	pool     chunkenc.Pool
	parquets map[string]*parquet.GenericReader[parquet.Page] // TODO Maybe any instead?
	size     int64
}

// NewColumnarChunkReader creates a new ColumnarChunkReader for the columnar format.
func NewColumnarChunkReader(dir string, pool chunkenc.Pool) (*ColumnarChunkReader, error) {
	cr := &ColumnarChunkReader{
		dir:      dir,
		pool:     pool,
		parquets: make(map[string]*parquet.GenericReader[parquet.Page]),
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var size int64

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".parquet") {
			// Add the file to the parquets map with a nil reader (lazy loading)
			cr.parquets[file.Name()] = nil

			fileInfo, err := file.Info()
			if err != nil {
				return nil, err
			}
			size += fileInfo.Size()
		}
	}

	cr.size = size
	return cr, nil
}

// ChunkOrIterable returns the chunk or iterable for the given chunk meta.
func (cr *ColumnarChunkReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	// TODO(jesus.vazquez) We need to find a way to link chunk refs to the chunks within parquet files.
	// IIRC the chunk references are ever increasing integers so maybe we can use that as an index into the parquet files.
	// The typical call sequence is:
	//
	//  var chks []chunks.Meta{}
	//  _ = Series(seriesRef, &builder, &chks)
	//  for _, chk := range chks {
	//    c, iterable, err := chunkr.ChunkOrIterable(chk)
	//    c.Bytes()
	//  }
	//
	// So the ColumnarIndexReader is responsible for building the chunk metas. We probably need to implement that method before this one.

	// TODO(jesus.vazquez) I think iterable should always be nil because we're reading from a block, not a WAL or head
	return nil, nil, errors.New("not implemented: ChunkOrIterable")
}

func (cr *ColumnarChunkReader) Size() int64 {
	return cr.size
}

// Close closes all open parquet readers.
func (cr *ColumnarChunkReader) Close() error {
	var errs []error

	for _, reader := range cr.parquets {
		if reader != nil {
			if err := reader.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing parquet readers: %v", errs)
	}

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
