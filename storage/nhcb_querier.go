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
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// NHCBAsClassicLabel is the control label that configures NHCB-to-classic
	// conversion per selector when NHCBAsClassicQuerier is active.
	//
	// Matchers on this label are stripped before querying the underlying storage:
	//   - "true" (or != "false"): enables NHCB-to-classic conversion for the selector.
	//   - "false" (or != "true", = ""): disables conversion and returns stored
	//     classic series unchanged.
	//   - "debug" (or =~ "true|debug"): enables conversion and attaches
	//     FromNHCBLabel ("true" for converted NHCB series, "false" for stored
	//     classic series) to all returned series. Stored and converted series
	//     are neither merged into a single labelset nor shadowed by each other,
	//     so both sources are returned in full side by side.
	NHCBAsClassicLabel = "__nhcb_as_classic__"

	// FromNHCBLabel is the label added to returned series when a selector uses
	// debug mode (__nhcb_as_classic__="debug" or =~"true|debug"): "true" for
	// classic series converted from NHCB, and "false" for stored classic series.
	// Unlike NHCBAsClassicLabel, it is a regular label on returned series;
	// matchers on FromNHCBLabel are passed through to the underlying storage.
	FromNHCBLabel = "__from_nhcb__"
)

// errOnlyControlMatchers is returned when a selector's only non-empty matchers
// are on NHCBAsClassicLabel, because stripping them would leave an empty
// selector that selects all series from the underlying storage.
var errOnlyControlMatchers = fmt.Errorf("vector selector must contain at least one non-empty matcher besides %s", NHCBAsClassicLabel)

// Known limitations of the NHCB-to-classic conversion:
//
// 1. TODO: This does not support the series API (LabelNames, LabelValues, etc.).
//    Only the Select method is wrapped. Any metadata or label introspection
//    queries will not reflect the converted classic series.

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

// NHCBAsClassicQueryable wraps a Queryable and applies NHCB-to-classic
// conversion to its queriers.
//
// NOTE: This is meant to wrap the Queryable used by the PromQL engine (see
// promql.EngineOpts.EnableNHCBAsClassic), above any fanout. Wrapping only one
// fanout leg (e.g. local storage) would leave remote-read secondaries
// unconverted and would also convert data served to non-PromQL consumers like
// the remote read API.
type NHCBAsClassicQueryable struct {
	Queryable
}

// NewNHCBAsClassicQueryable returns a new queryable that wraps the given
// queryable and applies NHCB-to-classic conversion to queriers.
func NewNHCBAsClassicQueryable(q Queryable) Queryable {
	return &NHCBAsClassicQueryable{Queryable: q}
}

// Querier implements the Queryable interface.
func (s *NHCBAsClassicQueryable) Querier(mint, maxt int64) (Querier, error) {
	q, err := s.Queryable.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return NewNHCBAsClassicQuerier(q), nil
}

// Select implements the Querier interface.
func (q *NHCBAsClassicQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	strippedMatchers, convert, debug, matched, err := extractControlMatchers(matchers)
	if err != nil {
		return ErrSeriesSet(err)
	}
	if !matched {
		return NoopSeriesSet()
	}

	nameMatcher, suffix, baseMatchers, leMatchers := extractHistogramSuffix(strippedMatchers)
	if suffix == "" || !convert {
		// Not a classic histogram query, or conversion explicitly disabled.
		return q.selectUnconverted(ctx, sortSeries, hints, debug, strippedMatchers)
	}

	baseNameMatcher := newBaseNameMatcher(nameMatcher.Type, nameMatcher.Value, suffix)
	if baseNameMatcher == nil {
		return q.selectUnconverted(ctx, sortSeries, hints, debug, strippedMatchers)
	}

	// Reuse baseMatchers' spare capacity to append baseNameMatcher without allocating.
	nhcbMatchers := append(baseMatchers, baseNameMatcher)
	nhcbSet := q.Querier.Select(ctx, sortSeries, hints, nhcbMatchers...)
	if nhcbSet.Err() != nil {
		return nhcbSet
	}

	// Query stored classic series without le matchers so that we can detect if
	// a stored classic histogram exists at a given timestamp even when the
	// query's le matcher only selects a subset of buckets (or a bucket label
	// that differs between stored classic and converted NHCB).
	//
	// NOTE: Both Select calls on q.Querier must happen before any SeriesSet.Next()
	// call, and advancing nhcbSet/classicSet is deferred to lazySeriesSet.Next(),
	// because secondaryQuerier (used by Fanout / NewMergeQuerier) panics if Select
	// is invoked after the first Next() of any returned SeriesSet.
	classicMatchers := strippedMatchers
	if len(leMatchers) > 0 {
		classicMatchers = make([]*labels.Matcher, 0, len(baseMatchers)+1)
		classicMatchers = append(classicMatchers, baseMatchers...)
		classicMatchers = append(classicMatchers, nameMatcher)
	}
	classicSet := q.Querier.Select(ctx, sortSeries, hints, classicMatchers...)
	if classicSet.Err() != nil {
		return classicSet
	}

	return &lazySeriesSet{init: func() SeriesSet {
		return selectNHCBAsClassic(ctx, sortSeries, nhcbSet, classicSet, leMatchers, suffix, debug)
	}}
}

func selectNHCBAsClassic(ctx context.Context, sortSeries bool, nhcbSet, classicSet SeriesSet, leMatchers []*labels.Matcher, suffix string, debug bool) SeriesSet {
	var (
		firstNHCB Series
		chkIter   chunkenc.Iterator
		hScratch  histogram.Histogram
		fhScratch histogram.FloatHistogram
	)
	for nhcbSet.Next() {
		s := nhcbSet.At()
		var ok bool
		if ok, chkIter = isNHCBSeries(s, chkIter, &hScratch, &fhScratch); !ok {
			continue
		}
		firstNHCB = s
		break
	}
	if err := nhcbSet.Err(); err != nil {
		return ErrSeriesSet(err)
	}

	// Fast path 1: when no NHCB series exist for the base metric name, stream
	// directly from the underlying classic SeriesSet.
	if firstNHCB == nil {
		if len(leMatchers) > 0 {
			classicSet = &leFilterSeriesSet{SeriesSet: classicSet, leMatchers: leMatchers}
		}
		if debug {
			classicSet = newFromNHCBSeriesSet(classicSet, "false")
		}
		if w := nhcbSet.Warnings(); len(w) > 0 {
			return &warningsSeriesSet{SeriesSet: classicSet, warnings: w}
		}
		return classicSet
	}

	var firstClassic Series
	for classicSet.Next() {
		if s := classicSet.At(); s != nil {
			firstClassic = s
			break
		}
	}
	if err := classicSet.Err(); err != nil {
		return ErrSeriesSet(err)
	}

	var warnings annotations.Annotations
	warnings.Merge(classicSet.Warnings())

	// Fast path 2: when no stored classic series exist and either sortSeries is
	// false or suffix is _count/_sum (which has no le label and therefore
	// preserves nhcbSet's sort order), stream NHCB series directly from nhcbSet
	// one series at a time without buffering all series up front.
	if firstClassic == nil && (!sortSeries || suffix != histogram.ClassicSuffixBucket) {
		return &nhcbToClassicSeriesSet{
			ctx:        ctx,
			firstNHCB:  firstNHCB,
			nhcbSet:    nhcbSet,
			leMatchers: leMatchers,
			suffix:     suffix,
			debug:      debug,
			warnings:   warnings,
		}
	}

	warnings.Merge(nhcbSet.Warnings())

	var groups []histogramGroup
	if firstClassic == nil {
		// sortSeries == true for a _bucket query with no stored classic series:
		// collect NHCB series so converted buckets can be sorted globally.
		groups = append(groups, histogramGroup{nhcb: []Series{firstNHCB}})
		for nhcbSet.Next() {
			s := nhcbSet.At()
			var ok bool
			if ok, chkIter = isNHCBSeries(s, chkIter, &hScratch, &fhScratch); !ok {
				continue
			}
			groups = append(groups, histogramGroup{nhcb: []Series{s}})
		}
		if err := nhcbSet.Err(); err != nil {
			return ErrSeriesSet(err)
		}
	} else {
		index := histogramIndex{stripLe: suffix == histogram.ClassicSuffixBucket}
		index.addClassic(firstClassic)
		for classicSet.Next() {
			if s := classicSet.At(); s != nil {
				index.addClassic(s)
			}
		}
		if err := classicSet.Err(); err != nil {
			return ErrSeriesSet(err)
		}
		index.addNHCB(firstNHCB)
		for nhcbSet.Next() {
			s := nhcbSet.At()
			var ok bool
			if ok, chkIter = isNHCBSeries(s, chkIter, &hScratch, &fhScratch); !ok {
				continue
			}
			index.addNHCB(s)
		}
		if err := nhcbSet.Err(); err != nil {
			return ErrSeriesSet(err)
		}
		groups = index.groups
	}

	return &nhcbToClassicSeriesSet{
		ctx:        ctx,
		groups:     groups,
		leMatchers: leMatchers,
		suffix:     suffix,
		sortSeries: sortSeries,
		debug:      debug,
		warnings:   warnings,
	}
}

// isNHCBSeries reports whether s is a candidate NHCB series (has no le label
// and its first sample is a custom-buckets histogram).
func isNHCBSeries(s Series, it chunkenc.Iterator, h *histogram.Histogram, fh *histogram.FloatHistogram) (bool, chunkenc.Iterator) {
	if s == nil || s.Labels().Has(labels.BucketLabel) {
		return false, it
	}
	it = s.Iterator(it)
	if it == nil {
		return false, nil
	}
	switch it.Next() {
	case chunkenc.ValHistogram:
		_, h = it.AtHistogram(h)
		return h != nil && histogram.IsCustomBucketsSchema(h.Schema), it
	case chunkenc.ValFloatHistogram:
		_, fh = it.AtFloatHistogram(fh)
		return fh != nil && histogram.IsCustomBucketsSchema(fh.Schema), it
	default:
		return false, it
	}
}

type lazySeriesSet struct {
	init func() SeriesSet
	set  SeriesSet
}

func (s *lazySeriesSet) Next() bool {
	if s.set == nil {
		s.set = s.init()
		s.init = nil
	}
	return s.set.Next()
}

func (s *lazySeriesSet) At() Series {
	if s.set == nil {
		return nil
	}
	return s.set.At()
}

func (s *lazySeriesSet) Err() error {
	if s.set == nil {
		return nil
	}
	return s.set.Err()
}

func (s *lazySeriesSet) Warnings() annotations.Annotations {
	if s.set == nil {
		return nil
	}
	return s.set.Warnings()
}

type leFilterSeriesSet struct {
	SeriesSet
	leMatchers []*labels.Matcher
}

func (s *leFilterSeriesSet) Next() bool {
	for s.SeriesSet.Next() {
		if matchesLe(s.SeriesSet.At().Labels(), s.leMatchers) {
			return true
		}
	}
	return false
}

// selectUnconverted selects series from the underlying Querier without
// NHCB-to-classic conversion. In debug mode, all returned series get
// FromNHCBLabel="false", because none of them were converted from NHCB.
//
// NOTE: Adding a constant label keeps sortSeries order, because every series
// has __name__, which sorts after FromNHCBLabel, so the label is always
// inserted before the first label that can differ between two series.
func (q *NHCBAsClassicQuerier) selectUnconverted(ctx context.Context, sortSeries bool, hints *SelectHints, debug bool, matchers []*labels.Matcher) SeriesSet {
	ss := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if debug {
		ss = newFromNHCBSeriesSet(ss, "false")
	}
	return ss
}

func isNHCBControlMatcher(m *labels.Matcher) bool {
	return m.Name == NHCBAsClassicLabel
}

func matchesAllControl(val string, ms []*labels.Matcher) bool {
	for _, m := range ms {
		if !m.Matches(val) {
			return false
		}
	}
	return true
}

// hasPositiveDebugMatcher reports whether at least one matcher positively
// selects "debug" without also matching the empty value or "false".
//
// NOTE: A negative matcher such as __nhcb_as_classic__!="true" or a broad
// wildcard such as __nhcb_as_classic__=~".+" matches the string "debug" under
// standard matcher semantics, but is intended to toggle conversion rather than
// enable debug label injection.
func hasPositiveDebugMatcher(ms []*labels.Matcher) bool {
	for _, m := range ms {
		if (m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp) &&
			m.Matches("debug") && !m.Matches("") && !m.Matches("false") {
			return true
		}
	}
	return false
}

// extractControlMatchers strips any NHCBAsClassicLabel matchers from matchers
// and evaluates whether conversion and debug mode are enabled for the selector.
// Conversion is enabled by default. When no NHCBAsClassicLabel matcher is
// present, matchers is returned as-is without allocating.
func extractControlMatchers(matchers []*labels.Matcher) (stripped []*labels.Matcher, convert, debug, matched bool, err error) {
	if !slices.ContainsFunc(matchers, isNHCBControlMatcher) {
		return matchers, true, false, true, nil
	}

	var controlMatchers []*labels.Matcher
	stripped = make([]*labels.Matcher, 0, len(matchers)-1)
	for _, m := range matchers {
		if isNHCBControlMatcher(m) {
			controlMatchers = append(controlMatchers, m)
		} else {
			stripped = append(stripped, m)
		}
	}
	if !slices.ContainsFunc(stripped, func(m *labels.Matcher) bool { return !m.Matches("") }) {
		return nil, false, false, false, errOnlyControlMatchers
	}

	matchTrue := matchesAllControl("true", controlMatchers)
	matchFalse := matchesAllControl("false", controlMatchers)
	matchEmpty := matchesAllControl("", controlMatchers)
	matchDebug := matchesAllControl("debug", controlMatchers) && hasPositiveDebugMatcher(controlMatchers)

	switch {
	case matchTrue && matchFalse && !matchDebug:
		return stripped, true, false, true, nil
	case matchFalse:
		return stripped, false, false, true, nil
	case matchTrue || matchDebug:
		return stripped, true, matchDebug, true, nil
	case matchEmpty:
		return stripped, false, false, true, nil
	default:
		return stripped, false, false, false, nil
	}
}

func withFromNHCB(lset labels.Labels, val string, b *labels.Builder) labels.Labels {
	if b == nil {
		b = labels.NewBuilder(lset)
	} else {
		b.Reset(lset)
	}
	b.Set(FromNHCBLabel, val)
	return b.Labels()
}

type relabeledSeries struct {
	Series
	lset labels.Labels
}

func (s relabeledSeries) Labels() labels.Labels { return s.lset }

type fromNHCBSeriesSet struct {
	SeriesSet
	val     string
	builder *labels.Builder
	cur     Series
}

func newFromNHCBSeriesSet(ss SeriesSet, val string) SeriesSet {
	return &fromNHCBSeriesSet{
		SeriesSet: ss,
		val:       val,
		builder:   labels.NewBuilder(labels.EmptyLabels()),
	}
}

func (s *fromNHCBSeriesSet) Next() bool {
	if !s.SeriesSet.Next() {
		s.cur = nil
		return false
	}
	ser := s.SeriesSet.At()
	if ser == nil {
		s.cur = nil
		return true
	}
	s.cur = relabeledSeries{
		Series: ser,
		lset:   withFromNHCB(ser.Labels(), s.val, s.builder),
	}
	return true
}

func (s *fromNHCBSeriesSet) At() Series { return s.cur }

type warningsSeriesSet struct {
	SeriesSet
	warnings annotations.Annotations
}

func (w *warningsSeriesSet) Warnings() annotations.Annotations {
	var out annotations.Annotations
	out.Merge(w.SeriesSet.Warnings())
	out.Merge(w.warnings)
	return out
}

// histogramGroup holds the stored classic series and NHCB series that share the
// same identifying labels (all labels except __name__, and le for _bucket queries).
type histogramGroup struct {
	id      labels.Labels
	classic []Series
	nhcb    []Series
}

// histogramIndex groups stored classic series and NHCB series by their
// histogram identity in insertion order.
type histogramIndex struct {
	stripLe bool
	groups  []histogramGroup
	byHash  map[uint64][]int
	scratch []byte
	builder labels.ScratchBuilder
}

func (idx *histogramIndex) groupFor(lset labels.Labels) *histogramGroup {
	if idx.byHash == nil {
		idx.byHash = make(map[uint64][]int)
		idx.builder = labels.NewScratchBuilder(0)
	}
	var (
		h uint64
		b []byte
	)
	if idx.stripLe {
		h, b = lset.HashWithoutLabels(idx.scratch, model.MetricNameLabel, labels.BucketLabel)
	} else {
		h, b = lset.HashWithoutLabels(idx.scratch, model.MetricNameLabel)
	}
	idx.scratch = b
	for _, i := range idx.byHash[h] {
		if equalGroupID(idx.groups[i].id, lset, idx.stripLe) {
			return &idx.groups[i]
		}
	}
	idx.builder.Reset()
	lset.Range(func(l labels.Label) {
		if l.Name != model.MetricNameLabel && (!idx.stripLe || l.Name != labels.BucketLabel) {
			idx.builder.Add(l.Name, l.Value)
		}
	})
	id := idx.builder.Labels()
	pos := len(idx.groups)
	idx.groups = append(idx.groups, histogramGroup{id: id})
	idx.byHash[h] = append(idx.byHash[h], pos)
	return &idx.groups[pos]
}

func (idx *histogramIndex) addClassic(s Series) {
	g := idx.groupFor(s.Labels())
	g.classic = append(g.classic, s)
}

func (idx *histogramIndex) addNHCB(s Series) {
	g := idx.groupFor(s.Labels())
	g.nhcb = append(g.nhcb, s)
}

// equalGroupID reports whether id equals lset after ignoring __name__ (and le
// when stripLe is true) on lset.
func equalGroupID(id, lset labels.Labels, stripLe bool) bool {
	var (
		expected []labels.Label
		actual   []labels.Label
	)
	id.Range(func(l labels.Label) {
		expected = append(expected, l)
	})
	lset.Range(func(l labels.Label) {
		if l.Name != model.MetricNameLabel && (!stripLe || l.Name != labels.BucketLabel) {
			actual = append(actual, l)
		}
	})
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return false
		}
	}
	return true
}

// histogramSuffix returns the classic histogram suffix (_bucket, _count, _sum)
// from the given metric name, or empty string if none matches.
func histogramSuffix(metricName string) string {
	switch {
	case strings.HasSuffix(metricName, histogram.ClassicSuffixBucket):
		return histogram.ClassicSuffixBucket
	case strings.HasSuffix(metricName, histogram.ClassicSuffixCount):
		return histogram.ClassicSuffixCount
	case strings.HasSuffix(metricName, histogram.ClassicSuffixSum):
		return histogram.ClassicSuffixSum
	default:
		return ""
	}
}

// newBaseNameMatcher creates a new __name__ matcher with the histogram suffix removed.
// Returns nil if the base name is empty or the matcher cannot be created.
func newBaseNameMatcher(matchType labels.MatchType, metricName, suffix string) *labels.Matcher {
	baseName := metricName[:len(metricName)-len(suffix)]
	if baseName == "" {
		return nil
	}
	m, err := labels.NewMatcher(matchType, model.MetricNameLabel, baseName)
	if err != nil {
		return nil
	}
	return m
}

// extractHistogramSuffix separates the equality __name__ matcher and any le
// matchers from the query matchers, and determines the classic histogram suffix
// (_bucket, _count, _sum).
//
// Only queries with an exact (__name__ = "<metric>_<suffix>") matcher are
// eligible for NHCB-to-classic conversion; regex or negative __name__ matchers
// cannot safely have a suffix stripped and are passed through unchanged.
func extractHistogramSuffix(matchers []*labels.Matcher) (*labels.Matcher, string, []*labels.Matcher, []*labels.Matcher) {
	var nameMatcher *labels.Matcher
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel && m.Type == labels.MatchEqual && nameMatcher == nil {
			nameMatcher = m
		}
	}
	if nameMatcher == nil {
		return nil, "", nil, nil
	}

	// Verify that every other __name__ matcher also matches nameMatcher.Value.
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel && !m.Matches(nameMatcher.Value) {
			return nil, "", nil, nil
		}
	}

	suffix := histogramSuffix(nameMatcher.Value)
	if suffix == "" {
		return nil, "", nil, nil
	}

	baseMatchers := make([]*labels.Matcher, 0, len(matchers))
	var leMatchers []*labels.Matcher
	for _, m := range matchers {
		switch {
		case m.Name == model.MetricNameLabel:
			continue
		case suffix == histogram.ClassicSuffixBucket && m.Name == labels.BucketLabel:
			leMatchers = append(leMatchers, m)
		default:
			baseMatchers = append(baseMatchers, m)
		}
	}

	return nameMatcher, suffix, baseMatchers, leMatchers
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

// nhcbToClassicSeriesSet converts NHCB series to classic histogram series
// format, resolving collisions with stored classic series per histogram group.
//
// When nhcbSet is non-nil (no stored classic series and no global bucket sort
// needed), NHCB series are streamed directly from nhcbSet one at a time.
// Otherwise, when sortSeries is false, groups are converted lazily one at a
// time as Next() advances; when sortSeries is true, all groups are converted
// on the first Next() call and sorted globally by labels.Compare.
type nhcbToClassicSeriesSet struct {
	ctx        context.Context
	firstNHCB  Series
	nhcbSet    SeriesSet
	groups     []histogramGroup
	groupIdx   int
	leMatchers []*labels.Matcher
	suffix     string
	sortSeries bool
	debug      bool
	warnings   annotations.Annotations

	initialized bool
	series      []Series
	idx         int
	err         error

	// Scratch state reused across series/groups.
	lsetBuilder *labels.Builder
	seriesCache histogram.ClassicSeriesCache
	builder     classicSeriesBuilder
	emitFn      func(labels.Labels, float64) error
	it          chunkenc.Iterator
	h           *histogram.Histogram
	fh          *histogram.FloatHistogram
}

func (s *nhcbToClassicSeriesSet) Next() bool {
	if s.err != nil {
		return false
	}

	if !s.initialized {
		s.initialized = true
		s.lsetBuilder = labels.NewBuilder(labels.EmptyLabels())
		s.emitFn = s.builder.emitSample

		if s.sortSeries {
			for i := range s.groups {
				if err := s.ctx.Err(); err != nil {
					s.err = err
					return false
				}
				groupSeries, err := s.convertGroup(&s.groups[i], nil)
				if err != nil {
					s.err = err
					return false
				}
				s.series = append(s.series, groupSeries...)
			}
			slices.SortFunc(s.series, func(a, b Series) int {
				return labels.Compare(a.Labels(), b.Labels())
			})
		}
	}

	if s.sortSeries {
		if s.idx < len(s.series) {
			s.idx++
			return true
		}
		return false
	}

	if s.idx < len(s.series) {
		s.idx++
		return true
	}

	// Streaming pure-NHCB path: pull one NHCB series at a time from nhcbSet.
	if s.nhcbSet != nil {
		for {
			if err := s.ctx.Err(); err != nil {
				s.err = err
				return false
			}
			var nhcbSeries Series
			if s.firstNHCB != nil {
				nhcbSeries = s.firstNHCB
				s.firstNHCB = nil
			} else {
				for s.nhcbSet.Next() {
					cand := s.nhcbSet.At()
					var ok bool
					if ok, s.it = isNHCBSeries(cand, s.it, s.h, s.fh); ok {
						nhcbSeries = cand
						break
					}
				}
				if nhcbSeries == nil {
					if err := s.nhcbSet.Err(); err != nil {
						s.err = err
					}
					return false
				}
			}

			converted, err := s.convertNHCBSeries(nhcbSeries, nil, s.series[:0])
			if err != nil {
				s.err = err
				return false
			}
			if len(converted) == 0 {
				continue
			}
			sortConvertedSeries(converted)
			s.series = converted
			s.idx = 1
			return true
		}
	}

	for s.groupIdx < len(s.groups) {
		if err := s.ctx.Err(); err != nil {
			s.err = err
			return false
		}
		g := &s.groups[s.groupIdx]
		s.groupIdx++

		groupSeries, err := s.convertGroup(g, s.series[:0])
		if err != nil {
			s.err = err
			return false
		}
		if len(groupSeries) == 0 {
			continue
		}
		s.series = groupSeries
		s.idx = 1
		return true
	}

	return false
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
	if s.nhcbSet == nil {
		return s.warnings
	}
	var w annotations.Annotations
	w.Merge(s.warnings)
	w.Merge(s.nhcbSet.Warnings())
	return w
}

func sortConvertedSeries(series []Series) {
	if len(series) <= 1 {
		return
	}
	slices.SortFunc(series, func(a, b Series) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
}

// convertGroup resolves a single histogram group into its output classic series.
func (s *nhcbToClassicSeriesSet) convertGroup(g *histogramGroup, dst []Series) ([]Series, error) {
	// Fast path: group has only stored classic series and no NHCB series.
	if len(g.nhcb) == 0 {
		out := dst[:0]
		for _, cs := range g.classic {
			if matchesLe(cs.Labels(), s.leMatchers) {
				if s.debug {
					cs = relabeledSeries{
						Series: cs,
						lset:   withFromNHCB(cs.Labels(), "false", s.lsetBuilder),
					}
				}
				out = append(out, cs)
			}
		}
		sortConvertedSeries(out)
		return out, nil
	}

	var (
		groupTS         []int64
		filteredClassic []Series
	)
	if len(g.classic) > 0 {
		// NOTE: In debug mode, stored classic and converted series get distinct
		// FromNHCBLabel values, so we skip shadowing NHCB samples at stored
		// classic timestamps to show both sources side by side.
		if !s.debug {
			var err error
			groupTS, s.it, err = collectClassicTimestamps(g.classic, s.it)
			if err != nil {
				return nil, err
			}
		}
		for _, cs := range g.classic {
			if matchesLe(cs.Labels(), s.leMatchers) {
				if s.debug {
					cs = relabeledSeries{
						Series: cs,
						lset:   withFromNHCB(cs.Labels(), "false", s.lsetBuilder),
					}
				}
				filteredClassic = append(filteredClassic, cs)
			}
		}
	}

	var converted []Series
	for i, nhcbSeries := range g.nhcb {
		if nhcbSeries == nil {
			continue
		}
		var seriesDst []Series
		if i == 0 && len(filteredClassic) == 0 {
			seriesDst = dst
		}
		seriesFromNHCB, err := s.convertNHCBSeries(nhcbSeries, groupTS, seriesDst)
		if err != nil {
			return nil, err
		}
		if len(converted) == 0 {
			converted = seriesFromNHCB
		} else if len(seriesFromNHCB) > 0 {
			converted, err = mergeSeriesByLabels(converted, seriesFromNHCB)
			if err != nil {
				return nil, err
			}
		}
	}

	out, err := mergeSeriesByLabels(filteredClassic, converted)
	if err != nil {
		return nil, err
	}
	sortConvertedSeries(out)
	return out, nil
}

// convertNHCBSeries converts a single NHCB series into classic series,
// shadowing samples at timestamps where the stored classic histogram is active.
func (s *nhcbToClassicSeriesSet) convertNHCBSeries(nhcbSeries Series, groupTS []int64, dst []Series) ([]Series, error) {
	nhcbLabels := nhcbSeries.Labels()
	if s.debug {
		nhcbLabels = withFromNHCB(nhcbLabels, "true", s.lsetBuilder)
	}
	s.it = nhcbSeries.Iterator(s.it)
	if s.it == nil {
		return nil, nil
	}

	s.builder.reset()
	tsIdx := 0

	for {
		valType := s.it.Next()
		if valType == chunkenc.ValNone {
			break
		}

		var (
			nhcb  any
			t     int64
			stale bool
		)

		switch valType {
		case chunkenc.ValHistogram:
			t, s.h = s.it.AtHistogram(s.h)
			if s.h == nil {
				continue
			}
			// Treat both explicit staleness markers and transitions to a
			// non-NHCB schema (e.g. exponential native histogram on the same
			// series) as ending any active converted NHCB series at t.
			if value.IsStaleNaN(s.h.Sum) || !histogram.IsCustomBucketsSchema(s.h.Schema) {
				stale = true
			} else {
				nhcb = s.h
			}
		case chunkenc.ValFloatHistogram:
			t, s.fh = s.it.AtFloatHistogram(s.fh)
			if s.fh == nil {
				continue
			}
			if value.IsStaleNaN(s.fh.Sum) || !histogram.IsCustomBucketsSchema(s.fh.Schema) {
				stale = true
			} else {
				nhcb = s.fh
			}
		case chunkenc.ValFloat:
			// NOTE: Any float sample on the NHCB series (e.g. a float StaleNaN
			// from scrape staleness or a type change to a float metric under the
			// base name) ends any active converted series at t.
			t = s.it.AtT()
			stale = true
		default:
			continue
		}

		if stale && len(s.builder.series) == 0 {
			continue
		}

		// If the stored classic histogram had a sample strictly between the
		// previous NHCB sample and t, mark any active converted series stale at
		// that classic takeover timestamp.
		for tsIdx < len(groupTS) && groupTS[tsIdx] < t {
			s.builder.shadow(groupTS[tsIdx])
			tsIdx++
		}

		if stale {
			s.builder.markAllStale(t)
			if tsIdx < len(groupTS) && groupTS[tsIdx] == t {
				tsIdx++
			}
			continue
		}

		// When the stored classic histogram has a live sample at timestamp t,
		// shadow the NHCB sample for the entire histogram group so that bucket
		// layout or le formatting differences cannot produce hybrid/duplicate
		// buckets at timestamp t.
		if tsIdx < len(groupTS) && groupTS[tsIdx] == t {
			s.builder.shadow(t)
			tsIdx++
			continue
		}

		s.builder.beginStep(t)
		if err := histogram.ConvertNHCBToClassic(nhcb, nhcbLabels, s.lsetBuilder, s.suffix, &s.seriesCache, s.emitFn); err != nil {
			return nil, err
		}
		s.builder.endStep(t)
	}

	if err := s.it.Err(); err != nil {
		return nil, err
	}

	// If the stored classic histogram has samples after the last NHCB sample,
	// mark any still-active converted series stale at the first such timestamp
	// so they do not linger across PromQL's lookback window.
	if tsIdx < len(groupTS) {
		s.builder.shadow(groupTS[tsIdx])
	}

	return s.builder.buildSeries(s.leMatchers, dst), nil
}

// mergeSeriesByLabels combines preferred (e.g. stored classic) and fallback
// (e.g. converted NHCB) series, merging any pair with identical labels via
// mergeSamples.
func mergeSeriesByLabels(preferred, fallback []Series) ([]Series, error) {
	if len(preferred) == 0 {
		return fallback, nil
	}
	if len(fallback) == 0 {
		return preferred, nil
	}

	out := make([]Series, 0, len(preferred)+len(fallback))
	usedFallback := make([]bool, len(fallback))
	for _, p := range preferred {
		pLabels := p.Labels()
		merged := p
		for i, f := range fallback {
			if !usedFallback[i] && labels.Equal(pLabels, f.Labels()) {
				usedFallback[i] = true
				var err error
				merged, err = mergeSamples(merged, f)
				if err != nil {
					return nil, err
				}
			}
		}
		out = append(out, merged)
	}
	for i, f := range fallback {
		if !usedFallback[i] {
			out = append(out, f)
		}
	}
	return out, nil
}

// collectClassicTimestamps returns the sorted, deduplicated timestamps of all
// non-stale float samples across the given classic series.
func collectClassicTimestamps(series []Series, it chunkenc.Iterator) ([]int64, chunkenc.Iterator, error) {
	var ts []int64
	for _, s := range series {
		if s == nil {
			continue
		}
		it = s.Iterator(it)
		if it == nil {
			continue
		}
		firstSeries := len(ts) == 0
		idx := 0
		needSort := false
		for it.Next() == chunkenc.ValFloat {
			t, f := it.At()
			if value.IsStaleNaN(f) {
				continue
			}
			if firstSeries {
				ts = append(ts, t)
				continue
			}
			for idx < len(ts) && ts[idx] < t {
				idx++
			}
			if idx < len(ts) && ts[idx] == t {
				idx++
				continue
			}
			ts = append(ts, t)
			needSort = true
		}
		if err := it.Err(); err != nil {
			return nil, it, err
		}
		if needSort {
			slices.Sort(ts)
			ts = slices.Compact(ts)
		}
	}
	return ts, it, nil
}

type sampleSource uint8

const (
	srcNone sampleSource = iota
	srcA
	srcB
)

// mergeSamples returns a Series that merges samples from a and b in timestamp
// order, preferring a (e.g. stored classic) when both have a sample at the same
// timestamp (unless a is a staleness marker and b has a live value).
func mergeSamples(a, b Series) (Series, error) {
	itA := a.Iterator(nil)
	itB := b.Iterator(nil)

	aSample, hasA := nextFloat(itA)
	bSample, hasB := nextFloat(itB)

	var (
		samples []fSample
		lastSrc sampleSource
	)
	appendSample := func(s fSample, src sampleSource) {
		if value.IsStaleNaN(s.f) {
			// Drop a staleness marker if no live sample has been emitted yet, or
			// if the series has already transitioned to the other source (e.g.
			// a delayed scrape staleness marker from the old representation
			// after the new representation started emitting samples).
			if lastSrc == srcNone || lastSrc != src {
				return
			}
			if len(samples) > 0 && value.IsStaleNaN(samples[len(samples)-1].f) {
				return
			}
		} else {
			lastSrc = src
		}
		if len(samples) > 0 && samples[len(samples)-1].t == s.t {
			samples[len(samples)-1] = s
			return
		}
		samples = append(samples, s)
	}

	for hasA && hasB {
		switch {
		case aSample.t < bSample.t:
			appendSample(aSample, srcA)
			aSample, hasA = nextFloat(itA)
		case bSample.t < aSample.t:
			appendSample(bSample, srcB)
			bSample, hasB = nextFloat(itB)
		default:
			// Same timestamp: prefer a (stored classic) unless it is a
			// staleness marker. If both are staleness markers, attribute the
			// marker to whichever source was active.
			switch {
			case !value.IsStaleNaN(aSample.f):
				appendSample(aSample, srcA)
			case !value.IsStaleNaN(bSample.f):
				appendSample(bSample, srcB)
			default:
				appendSample(aSample, lastSrc)
			}
			aSample, hasA = nextFloat(itA)
			bSample, hasB = nextFloat(itB)
		}
	}
	for hasA {
		appendSample(aSample, srcA)
		aSample, hasA = nextFloat(itA)
	}
	for hasB {
		appendSample(bSample, srcB)
		bSample, hasB = nextFloat(itB)
	}

	if err := itA.Err(); err != nil {
		return nil, err
	}
	if err := itB.Err(); err != nil {
		return nil, err
	}

	return &fSampleSeries{lset: a.Labels(), samples: samples}, nil
}

func nextFloat(it chunkenc.Iterator) (fSample, bool) {
	if it == nil {
		return fSample{}, false
	}
	for {
		switch it.Next() {
		case chunkenc.ValNone:
			return fSample{}, false
		case chunkenc.ValFloat:
			t, f := it.At()
			return fSample{t: t, f: f}, true
		}
	}
}

type convertedSeriesData struct {
	labels     labels.Labels
	samples    []fSample
	lastStep   int
	lastActive bool
}

// classicSeriesBuilder accumulates converted classic series samples for a
// single NHCB series, emitting staleness markers when a bucket disappears,
// when the NHCB sample is stale, or when a stored classic histogram shadows
// the NHCB series.
type classicSeriesBuilder struct {
	series  []convertedSeriesData
	byLabel map[uint64][]int
	step    int
	emitIdx int
	currT   int64
}

func (b *classicSeriesBuilder) reset() {
	for i := range b.series {
		b.series[i].labels = labels.EmptyLabels()
		b.series[i].samples = b.series[i].samples[:0]
		b.series[i].lastStep = 0
		b.series[i].lastActive = false
	}
	b.series = b.series[:0]
	if len(b.byLabel) > 0 {
		clear(b.byLabel)
	}
	b.step = 0
	b.emitIdx = 0
}

func (b *classicSeriesBuilder) beginStep(t int64) {
	b.step++
	b.emitIdx = 0
	b.currT = t
}

func (b *classicSeriesBuilder) emitSample(l labels.Labels, val float64) error {
	b.addSample(l, b.currT, val)
	return nil
}

func (b *classicSeriesBuilder) addSample(l labels.Labels, t int64, val float64) {
	idx := -1
	switch {
	case b.emitIdx < len(b.series) && labels.Equal(b.series[b.emitIdx].labels, l):
		idx = b.emitIdx
		b.emitIdx++
	case b.step == 1:
		// On the first sample of an NHCB series, ConvertNHCBToClassic emits
		// distinct bucket/count/sum series in order, so no hash lookup is needed.
		b.emitIdx = len(b.series) + 1
	default:
		if b.byLabel == nil {
			b.byLabel = make(map[uint64][]int, len(b.series))
		}
		if len(b.byLabel) == 0 && len(b.series) > 0 {
			for i := range b.series {
				h := b.series[i].labels.Hash()
				b.byLabel[h] = append(b.byLabel[h], i)
			}
		}
		h := l.Hash()
		for _, candidate := range b.byLabel[h] {
			if labels.Equal(b.series[candidate].labels, l) {
				idx = candidate
				b.emitIdx = candidate + 1
				break
			}
		}
	}
	if idx == -1 {
		idx = len(b.series)
		if idx < cap(b.series) {
			b.series = b.series[:idx+1]
			b.series[idx].labels = l
			b.series[idx].samples = b.series[idx].samples[:0]
			b.series[idx].lastStep = 0
			b.series[idx].lastActive = false
		} else {
			b.series = append(b.series, convertedSeriesData{
				labels: l,
			})
		}
		if len(b.byLabel) > 0 {
			h := l.Hash()
			b.byLabel[h] = append(b.byLabel[h], idx)
		}
	}
	s := &b.series[idx]
	s.samples = append(s.samples, fSample{t: t, f: val})
	s.lastStep = b.step
	s.lastActive = true
}

func (b *classicSeriesBuilder) endStep(t int64) {
	if b.emitIdx == len(b.series) && len(b.byLabel) == 0 {
		return
	}
	staleVal := math.Float64frombits(value.StaleNaN)
	for i := range b.series {
		s := &b.series[i]
		if s.lastActive && s.lastStep != b.step {
			s.samples = append(s.samples, fSample{t: t, f: staleVal})
			s.lastActive = false
		}
	}
}

func (b *classicSeriesBuilder) markAllStale(t int64) {
	staleVal := math.Float64frombits(value.StaleNaN)
	for i := range b.series {
		s := &b.series[i]
		if s.lastActive {
			s.samples = append(s.samples, fSample{t: t, f: staleVal})
			s.lastActive = false
		}
	}
}

func (b *classicSeriesBuilder) shadow(t int64) {
	b.markAllStale(t)
}

func (b *classicSeriesBuilder) buildSeries(leMatchers []*labels.Matcher, dst []Series) []Series {
	if len(b.series) == 0 {
		return dst[:0]
	}
	matchCount := 0
	totalSamples := 0
	for i := range b.series {
		s := &b.series[i]
		if !matchesLe(s.labels, leMatchers) {
			continue
		}
		matchCount++
		totalSamples += len(s.samples)
	}
	if matchCount == 0 {
		return dst[:0]
	}

	samplesSlab := make([]fSample, totalSamples)
	seriesSlab := make([]fSampleSeries, matchCount)
	out := dst[:0]
	if cap(out) < matchCount {
		out = make([]Series, 0, matchCount)
	}

	sampleIdx := 0
	seriesIdx := 0
	for i := range b.series {
		s := &b.series[i]
		if len(leMatchers) > 0 && !matchesLe(s.labels, leMatchers) {
			continue
		}
		n := len(s.samples)
		seriesSamples := samplesSlab[sampleIdx : sampleIdx+n : sampleIdx+n]
		copy(seriesSamples, s.samples)
		sampleIdx += n

		seriesSlab[seriesIdx] = fSampleSeries{
			lset:    s.labels,
			samples: seriesSamples,
		}
		out = append(out, &seriesSlab[seriesIdx])
		seriesIdx++
	}
	return out
}

// fSampleSeries implements Series over a slice of fSample without boxing each
// sample into the chunks.Sample interface.
type fSampleSeries struct {
	lset    labels.Labels
	samples []fSample
}

func (s *fSampleSeries) Labels() labels.Labels { return s.lset }

func (s *fSampleSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	if fsi, ok := it.(*fSampleIterator); ok {
		fsi.samples = s.samples
		fsi.idx = -1
		return fsi
	}
	return &fSampleIterator{samples: s.samples, idx: -1}
}

type fSampleIterator struct {
	samples []fSample
	idx     int
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
	if it.idx >= len(it.samples) {
		return chunkenc.ValNone
	}
	if it.samples[it.idx].t >= t {
		return chunkenc.ValFloat
	}
	it.idx += sort.Search(len(it.samples)-it.idx, func(i int) bool {
		return it.samples[it.idx+i].t >= t
	})
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

func (it *fSampleIterator) AtT() int64 { return it.samples[it.idx].t }
func (*fSampleIterator) AtST() int64   { return 0 }
func (*fSampleIterator) Err() error    { return nil }
