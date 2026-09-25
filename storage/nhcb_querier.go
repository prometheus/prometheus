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

package storage

import (
	"context"
	"math"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// Known limitations of the NHCB-to-classic conversion:
//
// 1. TODO: This does not support the series API (LabelNames, LabelValues, etc.).
//    Only the Select method is wrapped. Any metadata or label introspection
//    queries will not reflect the converted classic series.
//
// 2. TODO: The results are not properly sorted. When multiple NHCB series with
//    different label values are converted, the output is grouped by the
//    original NHCB series rather than being globally sorted by labels.
//    For example, given two NHCB series with method="GET" and method="POST",
//    the output order would be:
//
//      http_request_duration_seconds_bucket{le="0.1", method="GET"}
//      http_request_duration_seconds_bucket{le="+Inf", method="GET"}
//      http_request_duration_seconds_bucket{le="0.1", method="POST"}
//      http_request_duration_seconds_bucket{le="+Inf", method="POST"}
//
//    But the correctly sorted order (lexicographic by labels) would be:
//
//      http_request_duration_seconds_bucket{le="+Inf", method="GET"}
//      http_request_duration_seconds_bucket{le="+Inf", method="POST"}
//      http_request_duration_seconds_bucket{le="0.1", method="GET"}
//      http_request_duration_seconds_bucket{le="0.1", method="POST"}

// NHCBAsClassicQuerier wraps a Querier and converts NHCB (Native Histogram Custom Buckets)
// queries to classic histogram format when classic series don't exist.
type NHCBAsClassicQuerier struct {
	Querier
}

// NewNHCBAsClassicQuerier returns a new querier that wraps the given querier
// and converts NHCB to classic histogram format for queries.
func NewNHCBAsClassicQuerier(q Querier) Querier {
	return &NHCBAsClassicQuerier{Querier: q}
}

// NHCBAsClassicStorage wraps a Storage and applies NHCB-to-classic conversion
// to queriers when enabled.
type NHCBAsClassicStorage struct {
	Storage
}

// NewNHCBAsClassicStorage returns a new storage that wraps the given storage
// and applies NHCB-to-classic conversion to queriers.
func NewNHCBAsClassicStorage(s Storage) Storage {
	return &NHCBAsClassicStorage{Storage: s}
}

// Querier implements the Storage interface.
func (s *NHCBAsClassicStorage) Querier(mint, maxt int64) (Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return NewNHCBAsClassicQuerier(q), nil
}

// Select implements the Querier interface.
func (q *NHCBAsClassicQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	nameMatcher, suffix, baseMatchers := extractHistogramSuffix(matchers)
	if suffix == "" {
		// Not a classic histogram query, pass through
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	metricNameMacher := newBaseNameMatcher(nameMatcher.Type, nameMatcher.Value, suffix)
	if metricNameMacher == nil {
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	classicSet := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if classicSet.Err() != nil {
		return classicSet
	}

	var classicSeries []Series
	for classicSet.Next() {
		classicSeries = append(classicSeries, classicSet.At())
	}

	if err := classicSet.Err(); err != nil {
		return ErrSeriesSet(err)
	}

	seriesSets := make([]SeriesSet, 0, 2)
	if len(classicSeries) > 0 {
		seriesSets = append(seriesSets, &bufferedSeriesSet{series: classicSeries, warnings: classicSet.Warnings()})
	}
	matchersWithoutLe := make([]*labels.Matcher, 0, len(matchers)-1)
	var leMatcher *labels.Matcher
	for _, matcher := range baseMatchers {
		if matcher.Name == labels.BucketLabel {
			leMatcher = matcher
		} else {
			matchersWithoutLe = append(matchersWithoutLe, matcher)
		}
	}

	matchersWithoutLe = append(matchersWithoutLe, metricNameMacher)
	nhcbSet := q.Querier.Select(ctx, sortSeries, hints, matchersWithoutLe...)
	if nhcbSet.Err() != nil {
		return nhcbSet
	}
	seriesSets = append(seriesSets, &nhcbToClassicSeriesSet{
		nhcbSet:   nhcbSet,
		leMatcher: leMatcher,
		suffix:    suffix,
	})

	return &multipleSeriesSet{
		seriesSet: seriesSets,
		idx:       0,
	}
}

// bufferedSeriesSet wraps a buffered list of series.
type bufferedSeriesSet struct {
	series   []Series
	idx      int
	warnings annotations.Annotations
}

func (b *bufferedSeriesSet) Next() bool {
	if b.idx < len(b.series) {
		b.idx++
		return true
	}
	return false
}

func (b *bufferedSeriesSet) At() Series {
	if b.idx == 0 || b.idx > len(b.series) {
		return nil
	}
	return b.series[b.idx-1]
}

func (*bufferedSeriesSet) Err() error {
	return nil
}

func (b *bufferedSeriesSet) Warnings() annotations.Annotations {
	return b.warnings
}

// histogramSuffix returns the classic histogram suffix (_bucket, _count, _sum)
// from the given metric name, or empty string if none matches.
func histogramSuffix(metricName string) string {
	switch {
	case strings.HasSuffix(metricName, "_bucket"):
		return "_bucket"
	case strings.HasSuffix(metricName, "_count"):
		return "_count"
	case strings.HasSuffix(metricName, "_sum"):
		return "_sum"
	default:
		return ""
	}
}

// newBaseNameMatcher creates a new __name__ matcher with the histogram suffix removed.
// Returns nil if the base name matcher cannot be created.
func newBaseNameMatcher(matchType labels.MatchType, metricName, suffix string) *labels.Matcher {
	baseName := metricName[:len(metricName)-len(suffix)]
	m, err := labels.NewMatcher(matchType, model.MetricNameLabel, baseName)
	if err != nil {
		return nil
	}
	return m
}

// extractHistogramSuffix separates the __name__ matcher from other matchers and
// determines the classic histogram suffix (_bucket, _count, _sum).
// Returns the __name__ matcher, the suffix, and the remaining matchers.
// Returns empty suffix if not a classic histogram query.
func extractHistogramSuffix(matchers []*labels.Matcher) (*labels.Matcher, string, []*labels.Matcher) {
	var nameMatcher *labels.Matcher
	baseMatchers := make([]*labels.Matcher, 0, len(matchers))

	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			nameMatcher = m
		} else {
			baseMatchers = append(baseMatchers, m)
		}
	}

	if nameMatcher == nil {
		return nil, "", matchers
	}

	suffix := histogramSuffix(nameMatcher.Value)
	if suffix == "" {
		return nil, "", matchers
	}

	return nameMatcher, suffix, baseMatchers
}

type multipleSeriesSet struct {
	seriesSet []SeriesSet
	idx       int
}

func (m *multipleSeriesSet) Next() bool {
	if m.idx >= len(m.seriesSet) {
		return false
	}
	if !m.seriesSet[m.idx].Next() {
		m.idx++
		return m.Next()
	}
	return true
}

func (m *multipleSeriesSet) At() Series {
	return m.seriesSet[m.idx].At()
}

func (m *multipleSeriesSet) Err() error {
	for _, ss := range m.seriesSet {
		if err := ss.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (m *multipleSeriesSet) Warnings() annotations.Annotations {
	var w annotations.Annotations
	for _, ss := range m.seriesSet {
		w.Merge(ss.Warnings())
	}
	return w
}

// nhcbToClassicSeriesSet converts NHCB series to classic histogram series format.
type nhcbToClassicSeriesSet struct {
	nhcbSet   SeriesSet
	leMatcher *labels.Matcher
	suffix    string
	series    []Series
	idx       int
	err       error
}

func (s *nhcbToClassicSeriesSet) Next() bool {
	if s.err != nil {
		return false
	}
	// Convert all native histogram series on the first Next() call. A single
	// native histogram results in multiple classic histogram series, so
	// nothing can be returned before the whole set has been consumed.
	if s.series == nil && !s.convert() {
		return false
	}
	if s.idx < len(s.series) {
		s.idx++
		return true
	}
	return false
}

// nativeHistogramSeries holds the samples of a native histogram series.
type nativeHistogramSeries struct {
	labels  labels.Labels
	samples []nativeHistogramSample
}

// nativeHistogramSample is a sample of a native histogram series. fh is the
// histogram to convert, nil if the sample is not converted, e.g. a staleness
// marker.
type nativeHistogramSample struct {
	t  int64
	fh *histogram.FloatHistogram
}

// convert drains the wrapped series set and converts the native histograms to
// classic histogram series. It reports whether it succeeded.
func (s *nhcbToClassicSeriesSet) convert() bool {
	s.series = make([]Series, 0)

	nhSeries, ok := s.readNativeHistograms()
	if !ok {
		return false
	}

	lsetBuilder := labels.NewBuilder(labels.EmptyLabels())
	b := newClassicSeriesBuilder()
	emit := b.emit
	for _, ns := range nhSeries {
		seriesCache := &histogram.ClassicSeriesCache{}
		b.startSeries()
		for _, smpl := range ns.samples {
			b.startSample(smpl.t)
			if smpl.fh != nil {
				if err := histogram.ConvertNHCBToClassic(smpl.fh, ns.labels, lsetBuilder, s.suffix, seriesCache, emit); err != nil {
					s.err = err
					return false
				}
			}
			b.endSample()
		}
	}

	for _, data := range b.series {
		if s.leMatcher != nil {
			// In case a le was provided we need to filter with it
			if !s.leMatcher.Matches(data.labels.Get(labels.BucketLabel)) {
				continue
			}
		}

		s.series = append(s.series, NewListSeries(data.labels, data.samples))
	}
	return true
}

// readNativeHistograms drains the wrapped series set and returns its series.
// It reports whether it succeeded.
func (s *nhcbToClassicSeriesSet) readNativeHistograms() ([]nativeHistogramSeries, bool) {
	var nhSeries []nativeHistogramSeries
	for s.nhcbSet.Next() {
		series := s.nhcbSet.At()
		if series == nil {
			continue
		}
		it := series.Iterator(nil)
		if it == nil {
			continue
		}

		ns := nativeHistogramSeries{labels: series.Labels()}
		for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
			smpl := nativeHistogramSample{t: it.AtT()}
			if valType == chunkenc.ValHistogram || valType == chunkenc.ValFloatHistogram {
				// This works for histograms with integer counts, too.
				if _, fh := it.AtFloatHistogram(nil); fh != nil && s.convertible(fh.Schema, fh.Sum) {
					smpl.fh = fh
				}
			}
			ns.samples = append(ns.samples, smpl)
		}
		if err := it.Err(); err != nil {
			s.err = err
			return nil, false
		}
		nhSeries = append(nhSeries, ns)
	}
	if err := s.nhcbSet.Err(); err != nil {
		s.err = err
		return nil, false
	}
	return nhSeries, true
}

func (s *nhcbToClassicSeriesSet) At() Series {
	if s.idx == 0 || s.idx > len(s.series) {
		return nil
	}
	return s.series[s.idx-1]
}

func (s *nhcbToClassicSeriesSet) Err() error {
	return s.err
}

func (s *nhcbToClassicSeriesSet) Warnings() annotations.Annotations {
	return s.nhcbSet.Warnings()
}

// convertible reports whether a native histogram sample with the given schema
// and sum is converted to classic histogram series.
func (*nhcbToClassicSeriesSet) convertible(schema int32, sum float64) bool {
	// Staleness markers are not converted, whatever their schema. The series
	// converted from the previous sample are marked stale instead.
	return !value.IsStaleNaN(sum) && histogram.IsCustomBucketsSchema(schema)
}

type convertedSeriesData struct {
	labels  labels.Labels
	samples []chunks.Sample
}

// classicSeriesBuilder collects the classic histogram series converted from
// native histograms, one native histogram sample after the other.
//
// A converted series is marked stale at the first sample of its native
// histogram that does not result in it anymore, e.g. because the native
// histogram went stale or its bucket layout changed, just like the scrape loop
// marks series stale that disappear from a target.
type classicSeriesBuilder struct {
	series []*convertedSeriesData
	// byHash indexes series by label hash rather than by Labels.String().
	byHash map[uint64][]int

	// t is the timestamp of the current sample.
	t int64
	// emitted and prevEmitted are the indices of the series emitted for the
	// current and for the previous sample of the current native histogram.
	emitted, prevEmitted []int
}

func newClassicSeriesBuilder() *classicSeriesBuilder {
	return &classicSeriesBuilder{byHash: make(map[uint64][]int)}
}

// startSeries prepares for the samples of the next native histogram series.
func (b *classicSeriesBuilder) startSeries() {
	b.prevEmitted = b.prevEmitted[:0]
}

// startSample prepares for the series converted from the sample at t.
func (b *classicSeriesBuilder) startSample(t int64) {
	b.t = t
	b.emitted = b.emitted[:0]
}

// emit appends the value v at the timestamp of the current sample to the series
// with labels l. It is the emitSeriesFn of the conversion functions.
func (b *classicSeriesBuilder) emit(l labels.Labels, v float64) error {
	h := l.Hash()
	idx := -1
	for _, candidate := range b.byHash[h] {
		if labels.Equal(b.series[candidate].labels, l) {
			idx = candidate
			break
		}
	}
	if idx == -1 {
		idx = len(b.series)
		b.byHash[h] = append(b.byHash[h], idx)
		b.series = append(b.series, &convertedSeriesData{
			labels:  l,
			samples: make([]chunks.Sample, 0),
		})
	}

	b.series[idx].samples = append(b.series[idx].samples, fSample{
		t: b.t,
		f: v,
	})
	b.emitted = append(b.emitted, idx)
	return nil
}

// endSample marks the series emitted for the previous sample, but not for the
// current one, stale.
func (b *classicSeriesBuilder) endSample() {
	for _, idx := range b.prevEmitted {
		if samples := b.series[idx].samples; samples[len(samples)-1].T() != b.t {
			b.series[idx].samples = append(samples, fSample{t: b.t, f: math.Float64frombits(value.StaleNaN)})
		}
	}
	b.emitted, b.prevEmitted = b.prevEmitted, b.emitted
}
