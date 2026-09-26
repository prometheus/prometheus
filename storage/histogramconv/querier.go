// Copyright The Prometheus Authors
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

package histogramconv

import (
	"context"
	"slices"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// NewQuerier returns a querier that wraps the given querier and converts
// between histogram representations in Select, from the representations in
// convertFrom:
//
//   - A selector for classic histogram series, i.e. with a metric name with a
//     _bucket, _count or _sum suffix, also returns the classic histogram series
//     converted from the native histograms of the base name, from NHCB and
//     NHE.
//   - A selector for any other metric name also returns the NHCB assembled
//     from the classic histogram series of that name, from Classic.
//
// Each selector is handled by exactly one of the two, and both read stored
// series only, so nothing is converted twice and all representations can be
// converted from at the same time.
//
// Where a selector reads a histogram both stored and converted, stored data
// wins: converted samples are dropped at the timestamps of the stored samples
// of the same histogram, and the remaining ones are merged into the stored
// series with the same labels.
//
// Matchers on the ConvertStoredAsLabel override convertFrom per selector, and
// select the representations of the stored samples to return, too. Matchers
// on the DebugStoredAsLabel that match "true" add the StoredAsLabel to the
// returned series. Both are removed before selecting from the wrapped
// querier.
//
// Known limitations:
//
//   - Only Select converts. LabelNames and LabelValues return stored data
//     only.
//   - Stored data only wins at the exact timestamps of its samples. Where a
//     histogram is stored in both representations at different timestamps,
//     e.g. ingested from different sources, the merged series alternates
//     between them.
//   - Converted series, and stored series whose samples are filtered by
//     representation, are buffered in memory before the first one is
//     returned.
func NewQuerier(q storage.Querier, convertFrom []Representation) storage.Querier {
	return &querier{Querier: q, convertFrom: newRepresentations(convertFrom...)}
}

type querier struct {
	storage.Querier

	convertFrom representations
}

// Select implements the storage.Querier interface.
func (q *querier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	sel, err := newSelector(matchers, q.convertFrom)
	switch {
	case err != nil:
		return storage.ErrSeriesSet(err)
	case sel.stored == 0:
		return storage.EmptySeriesSet()
	case sel.passThrough():
		return q.Querier.Select(ctx, sortSeries, hints, sel.matchers...)
	}
	return &seriesSet{ctx: ctx, q: q.Querier, hints: hints, sel: sel}
}

// series is a series returned by a seriesSet: a converted series, a stored
// series whose samples are filtered, or a stored series returned unchanged,
// whose samples are only read if needed.
type series struct {
	lset    labels.Labels
	samples []chunks.Sample
	// stored is the stored series returned unchanged, nil once its samples
	// are read into samples.
	stored storage.Series
}

// storageSeries returns s as a storage.Series.
func (s *series) storageSeries() storage.Series {
	if s.stored != nil {
		return s.stored
	}
	return storage.NewListSeries(s.lset, s.samples)
}

// seriesSet returns the stored and the converted series of a selector, sorted
// by labels. It selects and converts them on the first call of Next, as the
// samples of all the series to convert from are needed to convert any of
// them.
type seriesSet struct {
	ctx   context.Context
	q     storage.Querier
	hints *storage.SelectHints
	sel   selector

	// it, h and fh are reused to read the samples of stored series.
	it chunkenc.Iterator
	h  *histogram.Histogram
	fh *histogram.FloatHistogram

	loaded   bool
	series   []storage.Series
	idx      int
	err      error
	warnings annotations.Annotations
}

func (s *seriesSet) Next() bool {
	if !s.loaded {
		s.loaded = true
		s.series, s.err = s.load()
	}
	if s.err != nil || s.idx >= len(s.series) {
		return false
	}
	s.idx++
	return true
}

func (s *seriesSet) At() storage.Series {
	if s.idx == 0 || s.idx > len(s.series) {
		return nil
	}
	return s.series[s.idx-1]
}

func (s *seriesSet) Err() error { return s.err }

func (s *seriesSet) Warnings() annotations.Annotations { return s.warnings }

// load selects the stored series and the ones to convert from, converts them,
// and lets the stored data win.
func (s *seriesSet) load() ([]storage.Series, error) {
	stored, err := s.selectStored()
	if err != nil {
		return nil, err
	}
	var converted []*series
	if s.sel.from != 0 {
		if converted, err = s.convert(); err != nil {
			return nil, err
		}
	}
	if len(converted) > 0 {
		if converted, err = s.storedWins(stored, converted); err != nil {
			return nil, err
		}
	}

	out := make([]storage.Series, 0, len(stored)+len(converted))
	for _, ser := range stored {
		out = append(out, ser.storageSeries())
	}
	for _, ser := range converted {
		out = append(out, ser.storageSeries())
	}
	slices.SortFunc(out, func(a, b storage.Series) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	return out, nil
}

// selectStored returns the stored series the selector reads. Unless their
// samples are filtered by representation, they are returned unchanged.
func (s *seriesSet) selectStored() ([]*series, error) {
	var (
		out    []*series
		ss     = s.q.Select(s.ctx, false, s.hints, s.sel.matchers...)
		filter = s.sel.stored != allRepresentations || s.sel.debug
		split  = storedSplitter{stored: s.sel.stored, debug: s.sel.debug}
	)
	for ss.Next() {
		if !filter {
			out = append(out, &series{lset: ss.At().Labels(), stored: ss.At()})
			continue
		}
		parts, err := split.split(ss.At())
		if err != nil {
			return nil, err
		}
		out = append(out, parts...)
	}
	s.warnings.Merge(ss.Warnings())
	return out, ss.Err()
}

// convert selects the series to convert from, and converts them.
func (s *seriesSet) convert() ([]*series, error) {
	var (
		sources   = s.q.Select(s.ctx, false, s.hints, s.sel.sourceMatchers...)
		converted []*series
		err       error
	)
	if s.sel.suffix != "" {
		converted, err = toClassic(sources, s.sel.suffix, s.sel.from, s.sel.leMatchers, s.sel.debug)
	} else {
		var ws annotations.Annotations
		converted, ws, err = toNHCB(sources, s.sel.debug)
		s.warnings.Merge(ws)
	}
	s.warnings.Merge(sources.Warnings())
	return converted, err
}
