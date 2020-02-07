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
	ChunkIteratorFn  func() chunks.Iterator
}

func NewTestSeries(lset labels.Labels, samples ...[]tsdbutil.Sample) Series {
	var chks []chunks.Meta

	return &ConcreteSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			var list tsdbutil.SampleSlice
			for _, l := range samples {
				list = append(list, l...)
			}
			return NewSampleIterator(list)
		},
		ChunkIteratorFn: func() chunks.Iterator {
			// Inefficient chunks encoding implementation, not caring about chunk size.
			for _, s := range samples {
				chks = append(chks, tsdbutil.ChunkFromSamples(s))
			}
			return NewChunksIterator(chks...)
		},
	}
}

func NewSeriesFromSamples(lset labels.Labels, samples tsdbutil.Samples) Series {
	return &ConcreteSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			return NewSampleIterator(samples)
		},
		ChunkIteratorFn: func() chunks.Iterator {
			// Inefficient chunks encoding implementation, not caring about chunk size.
			return NewChunksIterator(tsdbutil.ChunkFromSamplesGeneric(samples))
		},
	}
}

func (s *ConcreteSeries) Labels() labels.Labels             { return s.labels }
func (s *ConcreteSeries) SampleIterator() chunkenc.Iterator { return s.SampleIteratorFn() }
func (s *ConcreteSeries) ChunkIterator() chunks.Iterator    { return s.ChunkIteratorFn() }

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

//func (it *ChunkSeriesIterator) Seek(t int64) bool {
//	if it.idx == -1 {
//		it.idx = 0
//	}
//	// Do binary search between current position and end.
//	it.idx = sort.Search(len(it.samples)-it.idx, func(i int) bool {
//		s := it.samples[i+it.idx]
//		return s.t >= t
//	})
//
//	return it.idx < len(it.samples)
//}

type ChunkReader interface {
	Chunk(ref uint64) (chunkenc.Chunk, error)
}

type ChunkSeriesIterator struct {
	chks []chunks.Meta

	idx int
}

func NewPopulatedChunksIterator(chks ...chunks.Meta) chunks.Iterator {
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

func NewSeriesFromNotPopulatedChunks(lset labels.Labels, chks ...chunks.Meta) Series {
	return &ConcreteSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			panic("todo")
		},
		ChunkIteratorFn: func() chunks.Iterator {
			// Inefficient chunks encoding implementation, not caring about chunk size.
			return NewChunksIterator(tsdbutil.ChunkFromSamplesGeneric(samples))
		},
	}
}

//type ChunkSeriesIterator struct {
//	chks []chunks.Meta
//	idx  int
//}
//
//func NewChunksIterator(chks ...chunks.Meta) chunks.Iterator {
//	return &ChunkSeriesIterator{chks: chks, idx: -1}
//}
//
//func (it *ChunkSeriesIterator) At() chunks.Meta {
//	return it.chks[it.idx]
//}
//
//func (it *ChunkSeriesIterator) Next() bool {
//	it.idx++
//	return it.idx < len(it.chks)
//}
//
//func (it *ChunkSeriesIterator) Err() error { return nil }

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
