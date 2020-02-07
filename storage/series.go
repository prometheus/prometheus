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

import (
	"math"
	"sort"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type ConcreteSeries struct {
	labels           labels.Labels
	SampleIteratorFn func() chunkenc.Iterator
}

func NewTestSeries(lset labels.Labels, samples ...[]tsdbutil.Sample) *ConcreteSeries {
	return &ConcreteSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			var list tsdbutil.SampleSlice
			for _, l := range samples {
				list = append(list, l...)
			}
			return NewSampleIterator(list)
		},
	}
}

func NewSeriesFromSamples(lset labels.Labels, samples tsdbutil.Samples) Series {
	return &ConcreteSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			return NewSampleIterator(samples)
		},
	}
}

func (s *ConcreteSeries) Labels() labels.Labels       { return s.labels }
func (s *ConcreteSeries) Iterator() chunkenc.Iterator { return s.SampleIteratorFn() }

type SampleSeriesIterator struct {
	samples tsdbutil.Samples
	idx     int
}

func NewSampleIterator(samples tsdbutil.Samples) chunkenc.Iterator {
	return &SampleSeriesIterator{samples: samples, idx: -1}
}

func (it *SampleSeriesIterator) At() (int64, float64) {
	s := it.samples.Get(it.idx)
	return s.T(), s.V()
}

func (it *SampleSeriesIterator) Next() bool {
	it.idx++
	return it.idx < it.samples.Len()
}

func (it *SampleSeriesIterator) Seek(t int64) bool {
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

func (it *SampleSeriesIterator) Err() error { return nil }

type ChunkConcreteSeries struct {
	labels          labels.Labels
	ChunkIteratorFn func() chunks.Iterator
}

func NewTestChunkSeries(lset labels.Labels, samples ...[]tsdbutil.Sample) *ChunkConcreteSeries {
	var chks []chunks.Meta

	return &ChunkConcreteSeries{
		labels: lset,
		ChunkIteratorFn: func() chunks.Iterator {
			// Inefficient chunks encoding implementation, not caring about chunk size.
			for _, s := range samples {
				chks = append(chks, tsdbutil.ChunkFromSamples(s))
			}
			return NewListChunkSeriesIterator(chks...)
		},
	}
}

func (s *ChunkConcreteSeries) Labels() labels.Labels     { return s.labels }
func (s *ChunkConcreteSeries) Iterator() chunks.Iterator { return s.ChunkIteratorFn() }

type ChunkReader interface {
	Chunk(ref uint64) (chunkenc.Chunk, error)
}

type ChunkSeriesIterator struct {
	chks []chunks.Meta

	idx int
}

func NewListChunkSeriesIterator(chks ...chunks.Meta) chunks.Iterator {
	return &ChunkSeriesIterator{chks: chks, idx: -1}
}

func (it *ChunkSeriesIterator) At() chunks.Meta {
	return it.chks[it.idx]
}

func (it *ChunkSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.chks)
}

func (it *ChunkSeriesIterator) Err() error { return nil }

func NewListChunkSeries(lset labels.Labels, chks ...chunks.Meta) ChunkSeries {
	return &ChunkConcreteSeries{
		labels: lset,
		ChunkIteratorFn: func() chunks.Iterator {
			// Inefficient chunks encoding implementation, not caring about chunk size.
			return NewListChunkSeriesIterator(chks...)
		},
	}
}

func ExpandSamples(iter chunkenc.Iterator) ([]tsdbutil.Sample, error) {
	var result []tsdbutil.Sample
	for iter.Next() {
		t, v := iter.At()
		// NaNs can't be compared normally, so substitute for another value.
		if math.IsNaN(v) {
			v = -42
		}
		result = append(result, sample{t, v})
	}
	return result, iter.Err()
}

func ExpandChunks(iter chunks.Iterator) ([]chunks.Meta, error) {
	var result []chunks.Meta
	for iter.Next() {
		result = append(result, iter.At())
	}
	return result, iter.Err()
}
