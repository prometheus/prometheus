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
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
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
		// Not a classic histogram query, pass through.
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	metricNameMatcher := newBaseNameMatcher(nameMatcher.Type, nameMatcher.Value, suffix)
	if metricNameMatcher == nil {
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	classicSet := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if classicSet.Err() != nil {
		return classicSet
	}

	var (
		leMatchers        []*labels.Matcher
		matchersWithoutLe = baseMatchers
	)
	if suffix == histogram.ClassicSuffixBucket {
		// Filter in place: extractHistogramSuffix allocates baseMatchers with
		// capacity len(matchers), leaving room to append metricNameMatcher.
		matchersWithoutLe = baseMatchers[:0]
		for _, matcher := range baseMatchers {
			if matcher.Name == labels.BucketLabel {
				leMatchers = append(leMatchers, matcher)
			} else {
				matchersWithoutLe = append(matchersWithoutLe, matcher)
			}
		}
	}

	matchersWithoutLe = append(matchersWithoutLe, metricNameMatcher)
	nhcbSet := q.Querier.Select(ctx, sortSeries, hints, matchersWithoutLe...)
	if nhcbSet.Err() != nil {
		return nhcbSet
	}

	return &multipleSeriesSet{
		seriesSet: []SeriesSet{
			classicSet,
			&nhcbToClassicSeriesSet{
				nhcbSet:     nhcbSet,
				leMatchers:  leMatchers,
				suffix:      suffix,
				lsetBuilder: labels.NewBuilder(labels.EmptyLabels()),
			},
		},
	}
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
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			nameMatcher = m
		}
	}
	if nameMatcher == nil {
		return nil, "", matchers
	}

	suffix := histogramSuffix(nameMatcher.Value)
	if suffix == "" {
		return nil, "", matchers
	}

	baseMatchers := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		if m.Name != model.MetricNameLabel {
			baseMatchers = append(baseMatchers, m)
		}
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

// convertedClassicSeries accumulates converted float samples for a single classic series.
type convertedClassicSeries struct {
	labels  labels.Labels
	samples []fSample
}

// classicSeriesBuilder accumulates converted classic series for a single NHCB series
// while reusing slice/map capacity across NHCB series in the same SeriesSet.
type classicSeriesBuilder struct {
	series  []convertedClassicSeries
	byLabel map[uint64][]int
	currT   int64
	emitIdx int
}

func (b *classicSeriesBuilder) reset() {
	for i := range b.series {
		b.series[i].labels = labels.EmptyLabels()
		// Keep the backing sample slice allocated so subsequent NHCB series
		// reuse it without growing from 0.
		b.series[i].samples = b.series[i].samples[:0]
	}
	b.series = b.series[:0]
	clear(b.byLabel)
}

func (b *classicSeriesBuilder) beginTimestamp(t int64) {
	b.currT = t
	b.emitIdx = 0
}

func (b *classicSeriesBuilder) emitSample(l labels.Labels, val float64) error {
	b.addSample(l, b.currT, val)
	return nil
}

func (b *classicSeriesBuilder) addSample(l labels.Labels, t int64, val float64) int {
	// Fast path: when bucket layout has not changed across samples in this NHCB series,
	// ConvertNHCBToClassic emits buckets in the exact same order as b.series.
	idx := b.emitIdx
	b.emitIdx++
	if idx < len(b.series) && labels.Equal(b.series[idx].labels, l) {
		b.series[idx].samples = append(b.series[idx].samples, fSample{t: t, f: val})
		return idx
	}

	if idx == len(b.series) && len(b.byLabel) == 0 {
		b.growSeries(l)
		b.series[idx].samples = append(b.series[idx].samples, fSample{t: t, f: val})
		return idx
	}

	// Slow path fallback when bucket layout changes mid-series: populate byLabel lazily.
	if b.byLabel == nil {
		b.byLabel = make(map[uint64][]int, len(b.series)+4)
	}
	if len(b.byLabel) == 0 && len(b.series) > 0 {
		for i := range b.series {
			h := b.series[i].labels.Hash()
			b.byLabel[h] = append(b.byLabel[h], i)
		}
	}

	h := l.Hash()
	idx = -1
	for _, candidate := range b.byLabel[h] {
		if labels.Equal(b.series[candidate].labels, l) {
			idx = candidate
			break
		}
	}
	if idx == -1 {
		idx = len(b.series)
		b.byLabel[h] = append(b.byLabel[h], idx)
		b.growSeries(l)
	}
	b.series[idx].samples = append(b.series[idx].samples, fSample{t: t, f: val})
	return idx
}

func (b *classicSeriesBuilder) growSeries(l labels.Labels) {
	if len(b.series) < cap(b.series) {
		b.series = b.series[:len(b.series)+1]
		b.series[len(b.series)-1].labels = l
		b.series[len(b.series)-1].samples = b.series[len(b.series)-1].samples[:0]
		return
	}
	b.series = append(b.series, convertedClassicSeries{
		labels:  l,
		samples: make([]fSample, 0, 8),
	})
}

func matchesLe(lset labels.Labels, leMatchers []*labels.Matcher) bool {
	if len(leMatchers) == 0 {
		return true
	}
	le := lset.Get(labels.BucketLabel)
	for _, m := range leMatchers {
		if !m.Matches(le) {
			return false
		}
	}
	return true
}

func (b *classicSeriesBuilder) buildSeries(dst []Series, leMatchers []*labels.Matcher) []Series {
	matchCount := 0
	totalSamples := 0
	for i := range b.series {
		s := &b.series[i]
		if len(s.samples) == 0 || !matchesLe(s.labels, leMatchers) {
			continue
		}
		matchCount++
		totalSamples += len(s.samples)
	}
	if matchCount == 0 {
		return dst
	}

	// Allocate all output Series structs and their sample backing arrays in two
	// contiguous slabs per NHCB series rather than 2*matchCount separate allocations.
	seriesSlab := make([]fSampleSeries, matchCount)
	samplesSlab := make([]fSample, totalSamples)
	seriesIdx := 0
	sampleOffset := 0

	for i := range b.series {
		s := &b.series[i]
		if len(s.samples) == 0 || !matchesLe(s.labels, leMatchers) {
			continue
		}
		n := len(s.samples)
		samples := samplesSlab[sampleOffset : sampleOffset+n : sampleOffset+n]
		copy(samples, s.samples)
		sampleOffset += n

		seriesSlab[seriesIdx] = fSampleSeries{
			lset:    s.labels,
			samples: samples,
		}
		dst = append(dst, &seriesSlab[seriesIdx])
		seriesIdx++
	}
	return dst
}

// nhcbToClassicSeriesSet streams NHCB series and converts each one to classic
// histogram series on demand.
type nhcbToClassicSeriesSet struct {
	nhcbSet    SeriesSet
	leMatchers []*labels.Matcher
	suffix     string

	series []Series
	idx    int
	err    error

	lsetBuilder *labels.Builder
	seriesCache histogram.ClassicSeriesCache
	builder     classicSeriesBuilder
	emitFn      func(labels.Labels, float64) error
	hScratch    histogram.Histogram
	fhScratch   histogram.FloatHistogram
	chkIter     chunkenc.Iterator
}

func (s *nhcbToClassicSeriesSet) Next() bool {
	if s.err != nil {
		return false
	}
	if s.idx < len(s.series) {
		s.idx++
		return true
	}

	for s.nhcbSet.Next() {
		nhcbSeries := s.nhcbSet.At()
		if nhcbSeries == nil {
			continue
		}
		s.series = s.convertSeries(s.series[:0], nhcbSeries)
		if s.err != nil {
			return false
		}
		if len(s.series) > 0 {
			s.idx = 1
			return true
		}
	}

	if err := s.nhcbSet.Err(); err != nil {
		s.err = err
	}
	return false
}

func (s *nhcbToClassicSeriesSet) convertSeries(dst []Series, nhcbSeries Series) []Series {
	s.builder.reset()
	if s.emitFn == nil {
		s.emitFn = s.builder.emitSample
	}

	nhcbLabels := nhcbSeries.Labels()
	s.chkIter = nhcbSeries.Iterator(s.chkIter)
	it := s.chkIter
	if it == nil {
		return dst
	}

	for {
		valType := it.Next()
		if valType == chunkenc.ValNone {
			break
		}

		var (
			nhcb any
			t    int64
		)

		switch valType {
		case chunkenc.ValHistogram:
			var h *histogram.Histogram
			t, h = it.AtHistogram(&s.hScratch)
			if h == nil || !histogram.IsCustomBucketsSchema(h.Schema) {
				continue
			}
			nhcb = h
		case chunkenc.ValFloatHistogram:
			var fh *histogram.FloatHistogram
			t, fh = it.AtFloatHistogram(&s.fhScratch)
			if fh == nil || !histogram.IsCustomBucketsSchema(fh.Schema) {
				continue
			}
			nhcb = fh
		default:
			continue
		}

		s.builder.beginTimestamp(t)
		if err := histogram.ConvertNHCBToClassic(nhcb, nhcbLabels, s.lsetBuilder, s.suffix, &s.seriesCache, s.emitFn); err != nil {
			s.err = err
			return nil
		}
	}

	if err := it.Err(); err != nil {
		s.err = err
		return nil
	}

	return s.builder.buildSeries(dst, s.leMatchers)
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

// fSampleSeries implements Series backed by a contiguous []fSample slice,
// avoiding the per-sample interface boxing of []chunks.Sample and NewListSeries.
type fSampleSeries struct {
	lset    labels.Labels
	samples []fSample
}

func (s *fSampleSeries) Labels() labels.Labels { return s.lset }

func (s *fSampleSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	if fIt, ok := it.(*fSampleIterator); ok {
		fIt.reset(s.samples)
		return fIt
	}
	return &fSampleIterator{samples: s.samples, idx: -1}
}

type fSampleIterator struct {
	samples []fSample
	idx     int
}

func (it *fSampleIterator) reset(samples []fSample) {
	it.samples = samples
	it.idx = -1
}

func (it *fSampleIterator) Next() chunkenc.ValueType {
	it.idx++
	if it.idx >= len(it.samples) {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (it *fSampleIterator) Seek(t int64) chunkenc.ValueType {
	if it.idx < 0 {
		it.idx = 0
	}
	for it.idx < len(it.samples) && it.samples[it.idx].t < t {
		it.idx++
	}
	if it.idx >= len(it.samples) {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (it *fSampleIterator) At() (int64, float64) {
	s := it.samples[it.idx]
	return s.t, s.f
}

func (*fSampleIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("fSampleIterator does not contain histogram samples")
}

func (*fSampleIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("fSampleIterator does not contain float histogram samples")
}

func (it *fSampleIterator) AtT() int64 {
	return it.samples[it.idx].t
}

func (*fSampleIterator) AtST() int64 {
	return 0
}

func (*fSampleIterator) Err() error { return nil }
