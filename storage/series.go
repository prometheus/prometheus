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
	"sort"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type listSeriesIterator struct {
	samples []tsdbutil.Sample
	idx     int
}

// NewListSeriesIterator returns listSeriesIterator that allows to iterate over provided samples. Does not handle overlaps.
func NewListSeriesIterator(samples []tsdbutil.Sample) chunkenc.Iterator {
	return &listSeriesIterator{samples: samples, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.samples[it.idx]
	return s.T(), s.V()
}

func (it *listSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.samples)
}

func (it *listSeriesIterator) Seek(t int64) bool {
	if it.idx == -1 {
		it.idx = 0
	}
	// Do binary search between current position and end.
	it.idx = sort.Search(len(it.samples)-it.idx, func(i int) bool {
		s := it.samples[i+it.idx]
		return s.T() >= t
	})

	return it.idx < len(it.samples)
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
	bufIterator      chunkenc.Iterator
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
		c.sameSeriesChunks = append(c.sameSeriesChunks, &chunkToSeries{
			labels: c.ChunkSeriesSet.At().Labels(),
			chk:    iter.At(),
			buf:    c.bufIterator,
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

type chunkToSeries struct {
	labels labels.Labels
	chk    chunks.Meta
	buf    chunkenc.Iterator
}

func (s *chunkToSeries) Labels() labels.Labels       { return s.labels }
func (s *chunkToSeries) Iterator() chunkenc.Iterator { return s.chk.Chunk.Iterator(s.buf) }
