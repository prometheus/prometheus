// Copyright 2024 The Prometheus Authors

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

package series

import (
	"math"
	"slices"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/util/annotations"
)

type matchSet struct {
	data []*Data
	mu   sync.Mutex
}

func newMatchSet() *matchSet {
	return new(matchSet)
}

func (m *matchSet) Put(data *Data) {
	m.mu.Lock()
	m.data = append(m.data, data)
	m.mu.Unlock()
}

func (m *matchSet) Compile(db *pebble.DB, tenant uint64) storage.SeriesSet {
	m.mu.Lock()
	defer m.mu.Unlock()

	slices.SortFunc(m.data, Compare)

	series := map[uint64][]chunks.Sample{}

	var h prompb.Histogram
	for _, data := range m.data {
		for i := range data.ID {
			sa := &sample{t: int64(data.Timestamp[i])}
			switch encoding.SeriesKind(data.Kind[i]) {
			case encoding.SeriesFloat:
				sa.f = math.Float64frombits(data.Value[i])
			case encoding.SeriesHistogram:
				h.Reset()
				err := decodeHistogram(db, tenant, data.Value[i], h.Unmarshal)
				if err != nil {
					return storage.ErrSeriesSet(err)
				}
				sa.h = h.ToIntHistogram()
			case encoding.SeriesFloatHistogram:
				h.Reset()
				err := decodeHistogram(db, tenant, data.Value[i], h.Unmarshal)
				if err != nil {
					return storage.ErrSeriesSet(err)
				}
				sa.fh = h.ToFloatHistogram()
			}
			id := data.ID[i]
			series[id] = append(series[id], sa)
		}
	}

	lbl := make(map[uint64]labels.Labels)
	// We anticipate handling large amount of time series data. Allocating a slice
	// of all series id  only for sorting the data is wasteful.
	//
	// By using a bitmap we ensure to safely iterate over all the series in sorted order
	// as they were first processed.
	ra := roaring.NewBitmap()
	var ts prompb.TimeSeries
	for id := range series {
		lset, err := decodeLabels(&ts, db, tenant, id)
		if err != nil {
			return storage.ErrSeriesSet(err)
		}
		lbl[id] = lset
		_, _ = ra.Add(id)
	}

	it := ra.Iterator()
	it.Seek(0)

	return &seriesSet{
		series: series, labels: lbl, iter: it,
	}
}

type seriesSet struct {
	series map[uint64][]chunks.Sample
	labels map[uint64]labels.Labels
	iter   *roaring.Iterator
	id     uint64
}

var _ storage.SeriesSet = (*seriesSet)(nil)

func (s *seriesSet) Next() bool {
	id, eof := s.iter.Next()
	s.id = id
	return !eof
}

func (s *seriesSet) Err() error                        { return nil }
func (s *seriesSet) Warnings() annotations.Annotations { return nil }

func (s *seriesSet) At() storage.Series {
	return storage.NewListSeries(s.labels[s.id], s.series[s.id])
}

func decodeLabels(ts *prompb.TimeSeries, db *pebble.DB, tenant, id uint64) (labels.Labels, error) {
	ts.Reset()
	err := decodeLabelsBlob(db, tenant, id, ts.Unmarshal)
	if err != nil {
		return nil, err
	}
	lset := make(labels.Labels, len(ts.Labels))
	for i := range ts.Labels {
		la := &ts.Labels[i]
		lset[i] = labels.Label{
			Name: la.Name, Value: la.Value,
		}
	}
	return lset, nil
}

func decodeLabelsBlob(db *pebble.DB, tenant, id uint64, f func(val []byte) error) error {
	key := encoding.MakeBlobTranslationID(tenant, fields.Series.Hash(), id)
	val, done, err := db.Get(key)
	if err != nil {
		return err
	}
	defer done.Close()
	return f(val)
}

func decodeHistogram(db *pebble.DB, tenant, id uint64, f func(val []byte) error) error {
	key := encoding.MakeBlobTranslationID(tenant, fields.Histogram.Hash(), id)
	val, done, err := db.Get(key)
	if err != nil {
		return err
	}
	defer done.Close()
	return f(val)
}

type sample struct {
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
	t  int64
	f  float64
}

func (s sample) T() int64 {
	return s.t
}

func (s sample) F() float64 {
	return s.f
}

func (s sample) H() *histogram.Histogram {
	return s.h
}

func (s sample) FH() *histogram.FloatHistogram {
	return s.fh
}

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

func (s sample) Copy() chunks.Sample {
	c := sample{t: s.t, f: s.f}
	if s.h != nil {
		c.h = s.h.Copy()
	}
	if s.fh != nil {
		c.fh = s.fh.Copy()
	}
	return c
}
