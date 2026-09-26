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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
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
// Known limitations:
//
//   - Only Select converts. LabelNames and LabelValues return stored data
//     only.
//   - Converted series are returned in addition to the stored ones, even if
//     both exist for the same labels and timestamps, and they are returned
//     after the stored ones, rather than sorted.
//   - Converted series are buffered in memory before the first one is
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
	sel := newSelector(matchers, q.convertFrom)
	if sel.from == 0 {
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}
	return &seriesSet{ctx: ctx, q: q.Querier, sortSeries: sortSeries, hints: hints, sel: sel}
}

// series is a series with buffered samples, e.g. a converted one.
type series struct {
	lset    labels.Labels
	samples []chunks.Sample
}

// seriesSet returns the stored and the converted series of a selector. It
// selects and converts them on the first call of Next, as the samples of all
// the series to convert from are needed to convert any of them.
type seriesSet struct {
	ctx        context.Context
	q          storage.Querier
	sortSeries bool
	hints      *storage.SelectHints
	sel        selector

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

// load selects the stored series and the ones to convert from, and converts
// them.
func (s *seriesSet) load() ([]storage.Series, error) {
	var out []storage.Series
	stored := s.q.Select(s.ctx, s.sortSeries, s.hints, s.sel.matchers...)
	for stored.Next() {
		out = append(out, stored.At())
	}
	s.warnings.Merge(stored.Warnings())
	if err := stored.Err(); err != nil {
		return nil, err
	}

	var (
		sources   = s.q.Select(s.ctx, s.sortSeries, s.hints, s.sel.sourceMatchers...)
		converted []*series
		err       error
	)
	if s.sel.suffix != "" {
		converted, err = toClassic(sources, s.sel.suffix, s.sel.from, s.sel.leMatchers)
	} else {
		var ws annotations.Annotations
		converted, ws, err = toNHCB(sources)
		s.warnings.Merge(ws)
	}
	s.warnings.Merge(sources.Warnings())
	if err != nil {
		return nil, err
	}
	for _, c := range converted {
		out = append(out, storage.NewListSeries(c.lset, c.samples))
	}
	return out, nil
}
