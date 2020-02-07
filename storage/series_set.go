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

package storage

import "github.com/prometheus/prometheus/tsdb/chunkenc"

type chunkSetToSeriesSet struct {
	ChunkSeriesSet

	chkIterErr       error
	sameSeriesChunks []Series
	bufIterator      chunkenc.Iterator
}

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
		c.sameSeriesChunks = append(c.sameSeriesChunks, &ConcreteSeries{
			labels: c.ChunkSeriesSet.At().Labels(),
			SampleIteratorFn: func() chunkenc.Iterator {
				return iter.At().Chunk.Iterator(c.bufIterator)
			},
		})
	}

	if iter.Err() != nil {
		c.chkIterErr = iter.Err()
		return false
	}

	return true
}

func (c *chunkSetToSeriesSet) At() Series {
	return ChainedSeriesMerge(c.sameSeriesChunks...)
}

func (c *chunkSetToSeriesSet) Err() error {
	if c.chkIterErr != nil {
		return c.chkIterErr
	}
	return c.ChunkSeriesSet.Err()
}
