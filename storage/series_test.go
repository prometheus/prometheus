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

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type MockSeries struct {
	labels           labels.Labels
	SampleIteratorFn func() chunkenc.Iterator
}

func NewListSeries(lset labels.Labels, s []tsdbutil.Sample) *MockSeries {
	return &MockSeries{
		labels: lset,
		SampleIteratorFn: func() chunkenc.Iterator {
			return NewListSeriesIterator(samples(s))
		},
	}
}

func (s *MockSeries) Labels() labels.Labels       { return s.labels }
func (s *MockSeries) Iterator() chunkenc.Iterator { return s.SampleIteratorFn() }

type MockChunkSeries struct {
	labels          labels.Labels
	ChunkIteratorFn func() chunks.Iterator
}

func NewListChunkSeriesFromSamples(lset labels.Labels, samples ...[]tsdbutil.Sample) *MockChunkSeries {
	var chks []chunks.Meta

	return &MockChunkSeries{
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

func (s *MockChunkSeries) Labels() labels.Labels     { return s.labels }
func (s *MockChunkSeries) Iterator() chunks.Iterator { return s.ChunkIteratorFn() }

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
