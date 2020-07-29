// Copyright 2020 The Prometheus Authors
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
	"sort"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type listSeriesIterator struct {
	samples Samples
	idx     int
}

type samples []tsdbutil.Sample

func (s samples) Get(i int) tsdbutil.Sample { return s[i] }
func (s samples) Len() int                  { return len(s) }

// Samples interface allows to work on arrays of types that are compatible with tsdbutil.Sample.
type Samples interface {
	Get(i int) tsdbutil.Sample
	Len() int
}

// NewListSeriesIterator returns listSeriesIterator that allows to iterate over provided samples. Does not handle overlaps.
func NewListSeriesIterator(samples Samples) chunkenc.Iterator {
	return &listSeriesIterator{samples: samples, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.samples.Get(it.idx)
	return s.T(), s.V()
}

func (it *listSeriesIterator) Next() bool {
	it.idx++
	return it.idx < it.samples.Len()
}

func (it *listSeriesIterator) Seek(t int64) bool {
	if it.idx == -1 {
		it.idx = 0
	}
	// Do binary search between current position and end.
	it.idx = sort.Search(it.samples.Len()-it.idx, func(i int) bool {
		s := it.samples.Get(i + it.idx)
		return s.T() >= t
	})

	return it.idx < it.samples.Len()
}

func (it *listSeriesIterator) Err() error { return nil }

type listChunkSeriesIterator struct {
	chks []chunks.Meta

	idx int
}

// NewListChunkSeriesIterator returns listChunkSeriesIterator that allows to iterate over provided chunks. Does not handle overlaps.
func NewListChunkSeriesIterator(chks ...chunks.Meta) chunks.Iterator {
	return &listChunkSeriesIterator{chks: chks, idx: -1}
}

func (it *listChunkSeriesIterator) At() chunks.Meta {
	return it.chks[it.idx]
}

func (it *listChunkSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.chks)
}

func (it *listChunkSeriesIterator) Err() error { return nil }

type chunkSetToSeriesSet struct {
	ChunkSeriesSet

	chkIterErr       error
	sameSeriesChunks []Series
}

// NewSeriesSetFromChunkSeriesSet converts ChunkSeriesSet to SeriesSet by decoding chunks one by one.
func NewSeriesSetFromChunkSeriesSet(chk ChunkSeriesSet) SeriesSet {
	return &chunkSetToSeriesSet{ChunkSeriesSet: chk}
}

func (c *chunkSetToSeriesSet) Next() bool {
	if c.Err() != nil || !c.ChunkSeriesSet.Next() {
		return false
	}

	iter := c.ChunkSeriesSet.At().Iterator()
	c.sameSeriesChunks = c.sameSeriesChunks[:0]

	for iter.Next() {
		c.sameSeriesChunks = append(c.sameSeriesChunks, &chunkToSeriesDecoder{
			labels: c.ChunkSeriesSet.At().Labels(),
			Meta:   iter.At(),
		})
	}

	if iter.Err() != nil {
		c.chkIterErr = iter.Err()
		return false
	}

	return true
}

func (c *chunkSetToSeriesSet) At() Series {
	// Series composed of same chunks for the same series.
	return ChainedSeriesMerge(c.sameSeriesChunks...)
}

func (c *chunkSetToSeriesSet) Err() error {
	if c.chkIterErr != nil {
		return c.chkIterErr
	}
	return c.ChunkSeriesSet.Err()
}

type chunkToSeriesDecoder struct {
	chunks.Meta

	labels labels.Labels
}

func (s *chunkToSeriesDecoder) Labels() labels.Labels { return s.labels }

// TODO(bwplotka): Can we provide any chunkenc buffer?
func (s *chunkToSeriesDecoder) Iterator() chunkenc.Iterator { return s.Chunk.Iterator(nil) }

type seriesSetToChunkSet struct {
	SeriesSet
}

// NewSeriesSetToChunkSet converts SeriesSet to ChunkSeriesSet by encoding chunks from samples.
func NewSeriesSetToChunkSet(chk SeriesSet) ChunkSeriesSet {
	return &seriesSetToChunkSet{SeriesSet: chk}
}

func (c *seriesSetToChunkSet) Next() bool {
	if c.Err() != nil || !c.SeriesSet.Next() {
		return false
	}
	return true
}

func (c *seriesSetToChunkSet) At() ChunkSeries {
	return &seriesToChunkEncoder{
		Series: c.SeriesSet.At(),
	}
}

func (c *seriesSetToChunkSet) Err() error {
	return c.SeriesSet.Err()
}

type seriesToChunkEncoder struct {
	Series
}

// TODO(bwplotka): Currently encoder will just naively build one chunk, without limit. Split it: https://github.com/prometheus/tsdb/issues/670
func (s *seriesToChunkEncoder) Iterator() chunks.Iterator {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	if err != nil {
		return errChunksIterator{err: err}
	}
	mint := int64(math.MaxInt64)
	maxt := int64(math.MinInt64)

	seriesIter := s.Series.Iterator()
	for seriesIter.Next() {
		t, v := seriesIter.At()
		app.Append(t, v)

		maxt = t
		if mint == math.MaxInt64 {
			mint = t
		}
	}
	if err := seriesIter.Err(); err != nil {
		return errChunksIterator{err: err}
	}

	return NewListChunkSeriesIterator(chunks.Meta{
		MinTime: mint,
		MaxTime: maxt,
		Chunk:   chk,
	})
}

type errChunksIterator struct {
	err error
}

func (e errChunksIterator) At() chunks.Meta { return chunks.Meta{} }
func (e errChunksIterator) Next() bool      { return false }
func (e errChunksIterator) Err() error      { return e.err }
