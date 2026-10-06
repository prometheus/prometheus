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
	"strconv"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// OptLabelPrefix is the reserved prefix for per-selector query option
	// matchers (such as ClassicFromLabel). Matchers with this prefix are
	// stripped before querying storage in PromQL (even when conversion feature
	// flags are disabled) and omitted from absent() output, so rolling back a
	// feature flag does not break queries that carry option matchers.
	OptLabelPrefix = "__opt_"

	// ClassicFromLabel is the control label that configures which native
	// histogram schemas are converted to classic series per selector when
	// NHCBAsClassicQuerier is active.
	//
	// Matchers on this label are stripped before querying the underlying
	// storage:
	//   - "nhcb" (or != "nhe"): converts only Native Histograms with Custom
	//     Buckets (NHCB).
	//   - "nhe" (or != "nhcb"): converts only exponential native histograms
	//     (NHE).
	//   - =~ "nhcb|nhe": converts both NHCB and NHE (default when no
	//     ClassicFromLabel matcher is specified).
	//   - "none" or "" (or !~ "nhcb|nhe"): disables native-to-classic
	//     conversion for the selector and returns stored classic series
	//     unchanged.
	//   - "debug" (or =~ "...|debug", e.g. =~ "nhcb|debug", =~ "nhe|debug",
	//     =~ "nhcb|nhe|debug"): enables conversion (for both NHCB and NHE when
	//     "debug" is used alone, or for the matched schema(s)) and attaches
	//     StoredAsLabel ("classic", "nhcb", or "nhe") to all returned series
	//     without merging series of different storage forms into a single
	//     labelset or shadowing native samples at stored classic timestamps.
	//
	// Any other value in an = or != matcher returns an error.
	ClassicFromLabel = "__opt_classic_from"

	// StoredAsLabel is the label added to returned series when a selector uses
	// debug mode (__opt_classic_from="debug" or =~"...|debug"):
	//   - "classic" for stored classic series (and any other unconverted series),
	//   - "nhcb" for classic series converted from NHCB samples, and
	//   - "nhe" for classic series converted from exponential native histogram
	//     samples.
	//
	// Unlike ClassicFromLabel, it is a regular label on returned series. In
	// debug mode, matchers on StoredAsLabel are applied to the returned series
	// instead of the underlying storage, which never has this label. Otherwise,
	// they are passed through to the underlying storage.
	StoredAsLabel = "__stored_as__"

	// StoredAsClassic is the StoredAsLabel value for stored classic series.
	StoredAsClassic = "classic"
	// StoredAsNHCB is the StoredAsLabel value for series converted from NHCB.
	StoredAsNHCB = "nhcb"
	// StoredAsNHE is the StoredAsLabel value for series converted from
	// exponential native histograms.
	StoredAsNHE = "nhe"
)

// errOnlyControlMatchers is returned when a selector's only non-empty matchers
// are on __opt_* labels (or StoredAsLabel in debug mode), because stripping
// them would leave an empty selector that selects all series from the
// underlying storage.
var errOnlyControlMatchers = fmt.Errorf("vector selector must contain at least one non-empty matcher besides %s and %s", ClassicFromLabel, StoredAsLabel)

// errInvalidControlValue is returned when an equality or inequality matcher on
// ClassicFromLabel uses an unknown value, because silently returning no data
// for e.g. a typo would be hard to debug.
var errInvalidControlValue = fmt.Errorf(`invalid %s value, must be one of "nhcb", "nhe", "none" or "debug"`, ClassicFromLabel)

// Known limitations of the native-to-classic conversion:
//
//  1. TODO: This does not support the series API (LabelNames, LabelValues, etc.).
//     Only the Select method is wrapped. Any metadata or label introspection
//     queries will not reflect the converted classic series.
//  2. Exponential native histograms have no fixed bucket layout, so their
//     classic buckets are synthesized: either at the le values pinned by the
//     query or at the populated bucket bounds (capped at
//     exponentialAsClassicMaxSchema) unified across the series of a Select.
//     Bounds that do not coincide with an exponential bucket boundary are
//     interpolated, so le="0.1" style queries are estimates rather than exact
//     counts.

// NHCBAsClassicQuerier wraps a Querier and converts native histogram (NHCB and
// exponential) queries to classic histogram format when classic series don't
// exist.
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

// exponentialAsClassicMaxSchema caps the resolution at which exponential native
// histograms are expanded into classic buckets when the query does not pin
// specific le values. Schema 2 yields 4 buckets per power of two (factor
// ~1.19), which keeps linear interpolation in histogram_quantile close to the
// native estimate without exploding the number of synthesized series (schema 8
// would produce 256 buckets per power of two).
const exponentialAsClassicMaxSchema int32 = 2

// Select implements the Querier interface.
func (q *NHCBAsClassicQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	strippedMatchers, convertNHCB, convertNHE, debug, matched, err := extractControlMatchers(matchers)
	if err != nil {
		return ErrSeriesSet(err)
	}
	if !matched {
		// Contradictory control matchers (e.g. ="nhcb" and ="nhe") select nothing.
		return NoopSeriesSet()
	}
	if debug && slices.ContainsFunc(strippedMatchers, isStoredAsMatcher) {
		// NOTE: StoredAsLabel only exists on returned series in debug mode, so
		// its matchers are applied to the output instead of being pushed down.
		var storedAsMatchers, rest []*labels.Matcher
		for _, m := range strippedMatchers {
			if isStoredAsMatcher(m) {
				storedAsMatchers = append(storedAsMatchers, m)
			} else {
				rest = append(rest, m)
			}
		}
		if !slices.ContainsFunc(rest, func(m *labels.Matcher) bool { return !m.Matches("") }) {
			return ErrSeriesSet(errOnlyControlMatchers)
		}
		return &storedAsFilterSeriesSet{
			SeriesSet: q.selectConverted(ctx, sortSeries, hints, convertNHCB, convertNHE, debug, rest),
			matchers:  storedAsMatchers,
		}
	}
	return q.selectConverted(ctx, sortSeries, hints, convertNHCB, convertNHE, debug, strippedMatchers)
}

// selectConverted selects series for strippedMatchers (without control
// matchers), converting enabled native histogram schemas to classic series.
func (q *NHCBAsClassicQuerier) selectConverted(ctx context.Context, sortSeries bool, hints *SelectHints, convertNHCB, convertNHE, debug bool, strippedMatchers []*labels.Matcher) SeriesSet {
	nameMatcher, suffix, baseMatchers, leMatchers := extractHistogramSuffix(strippedMatchers)
	if suffix == "" || (!convertNHCB && !convertNHE) {
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
		return selectNHCBAsClassic(ctx, sortSeries, nhcbSet, classicSet, leMatchers, suffix, convertNHCB, convertNHE, debug)
	}}
}

func selectNHCBAsClassic(ctx context.Context, sortSeries bool, nhcbSet, classicSet SeriesSet, leMatchers []*labels.Matcher, suffix string, convertNHCB, convertNHE, debug bool) SeriesSet {
	var (
		firstNHCB     Series
		firstKind     nativeSeriesKind
		chkIter       chunkenc.Iterator
		hScratch      histogram.Histogram
		fhScratch     histogram.FloatHistogram
		exponentialOK = convertNHE && suffix == histogram.ClassicSuffixBucket
	)
	for nhcbSet.Next() {
		s := nhcbSet.At()
		firstKind, chkIter = classifyNativeSeries(s, chkIter, &hScratch, &fhScratch, convertNHCB, convertNHE)
		if firstKind == nativeSeriesNone {
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
			classicSet = newStoredAsSeriesSet(classicSet, StoredAsClassic)
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

	// Exponential native histograms have no fixed bucket layout, so classic
	// bounds must be chosen for them. If the query pins le values, those are
	// evaluated directly on every sample. Otherwise all native series are
	// buffered so that the populated bucket bounds can be unified across
	// series and time ("exponential mode"): emitting the same le set for
	// every series and sample is what keeps rate() and sum by (le) correct.
	var bounds []float64
	if exponentialOK {
		bounds = boundsFromLeMatchers(leMatchers)
	}
	exponentialOK = exponentialOK && bounds == nil

	// Fast path 2: when no stored classic series exist and either sortSeries is
	// false or suffix is _count/_sum (which has no le label and therefore
	// preserves nhcbSet's sort order unless debug mode can emit both "nhcb" and
	// "nhe" StoredAsLabel values), stream NHCB series directly from nhcbSet one
	// series at a time without buffering all series up front. The series set
	// switches to exponential mode itself once it meets the first exponential
	// series, buffering only the remaining ones.
	if firstClassic == nil && (!sortSeries || (suffix != histogram.ClassicSuffixBucket && (!debug || !convertNHCB || !convertNHE))) {
		return &nhcbToClassicSeriesSet{
			ctx:             ctx,
			firstNHCB:       firstNHCB,
			firstKind:       firstKind,
			nhcbSet:         nhcbSet,
			leMatchers:      leMatchers,
			suffix:          suffix,
			bounds:          bounds,
			exponentialMode: exponentialOK,
			convertNHCB:     convertNHCB,
			convertNHE:      convertNHE,
			debug:           debug,
			warnings:        warnings,
		}
	}

	warnings.Merge(nhcbSet.Warnings())

	var (
		groups         []histogramGroup
		anyExponential = firstKind == nativeSeriesExponential
	)
	if firstClassic == nil {
		// No stored classic series, but sortSeries == true for a _bucket
		// query (converted buckets must be sorted globally): collect NHCB
		// series.
		groups = append(groups, histogramGroup{nhcb: []Series{firstNHCB}})
		for nhcbSet.Next() {
			s := nhcbSet.At()
			var kind nativeSeriesKind
			if kind, chkIter = classifyNativeSeries(s, chkIter, &hScratch, &fhScratch, convertNHCB, convertNHE); kind == nativeSeriesNone {
				continue
			}
			anyExponential = anyExponential || kind == nativeSeriesExponential
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
			var kind nativeSeriesKind
			if kind, chkIter = classifyNativeSeries(s, chkIter, &hScratch, &fhScratch, convertNHCB, convertNHE); kind == nativeSeriesNone {
				continue
			}
			anyExponential = anyExponential || kind == nativeSeriesExponential
			index.addNHCB(s)
		}
		if err := nhcbSet.Err(); err != nil {
			return ErrSeriesSet(err)
		}
		groups = index.groups
	}

	if exponentialOK && anyExponential {
		var err error
		if bounds, err = unifiedExponentialBounds(ctx, groups, chkIter); err != nil {
			return ErrSeriesSet(err)
		}
	}

	return &nhcbToClassicSeriesSet{
		ctx:         ctx,
		groups:      groups,
		leMatchers:  leMatchers,
		suffix:      suffix,
		bounds:      bounds,
		sortSeries:  sortSeries,
		convertNHCB: convertNHCB,
		convertNHE:  convertNHE,
		debug:       debug,
		warnings:    warnings,
	}
}

// IsOptMatcher reports whether m is a per-selector query option matcher
// (having OptLabelPrefix).
func IsOptMatcher(m *labels.Matcher) bool {
	return strings.HasPrefix(m.Name, OptLabelPrefix)
}

// stripOptMatchers returns matchers without any OptLabelPrefix matchers.
// When none is present, matchers is returned as-is without allocating.
// If stripping leaves no non-empty matchers, errOnlyControlMatchers is returned.
func stripOptMatchers(matchers []*labels.Matcher) ([]*labels.Matcher, error) {
	if !slices.ContainsFunc(matchers, IsOptMatcher) {
		return matchers, nil
	}
	stripped := make([]*labels.Matcher, 0, len(matchers)-1)
	for _, m := range matchers {
		if !IsOptMatcher(m) {
			stripped = append(stripped, m)
		}
	}
	if !slices.ContainsFunc(stripped, func(m *labels.Matcher) bool { return !m.Matches("") }) {
		return nil, errOnlyControlMatchers
	}
	return stripped, nil
}

// optStripQuerier wraps a Querier and strips any OptLabelPrefix matchers in
// Select before forwarding calls. It is used by the PromQL engine when
// conversion feature flags are disabled so that __opt_* matchers never reach
// storage.
type optStripQuerier struct {
	Querier
}

// NewOptStripQuerier returns a Querier that strips OptLabelPrefix matchers in
// Select before delegating to q. If q already strips option matchers (such as
// NHCBAsClassicQuerier or optStripQuerier), q is returned as-is.
func NewOptStripQuerier(q Querier) Querier {
	switch q.(type) {
	case nil, noopQuerier, *NHCBAsClassicQuerier, *optStripQuerier:
		return q
	default:
		return &optStripQuerier{Querier: q}
	}
}

func (q *optStripQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	stripped, err := stripOptMatchers(matchers)
	if err != nil {
		return ErrSeriesSet(err)
	}
	return q.Querier.Select(ctx, sortSeries, hints, stripped...)
}

// selectUnconverted selects series from the underlying Querier without
// native-to-classic conversion. In debug mode, all returned series get
// StoredAsLabel="classic", because none of them were converted from native
// histograms.
//
// NOTE: Adding a constant StoredAsLabel preserves sortSeries order because
// every stored series has __name__ and no user labels sort between __name__ and
// __stored_as__.
func (q *NHCBAsClassicQuerier) selectUnconverted(ctx context.Context, sortSeries bool, hints *SelectHints, debug bool, matchers []*labels.Matcher) SeriesSet {
	ss := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if debug {
		ss = newStoredAsSeriesSet(ss, StoredAsClassic)
	}
	return ss
}

func isClassicFromMatcher(m *labels.Matcher) bool {
	return m.Name == ClassicFromLabel
}

func isStoredAsMatcher(m *labels.Matcher) bool {
	return m.Name == StoredAsLabel
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
// selects "debug" without also matching the empty value or "none".
//
// NOTE: A negative matcher such as __opt_classic_from!="nhe" or a broad
// wildcard such as __opt_classic_from=~".+" matches the string "debug" under
// standard matcher semantics, but is intended to select conversion schemas
// rather than enable debug label injection.
func hasPositiveDebugMatcher(ms []*labels.Matcher) bool {
	for _, m := range ms {
		if (m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp) &&
			m.Matches("debug") && !m.Matches("") && !m.Matches("none") {
			return true
		}
	}
	return false
}

// extractControlMatchers strips any OptLabelPrefix matchers from matchers
// and evaluates which native histogram schemas ("nhcb", "nhe") and debug mode
// are enabled for the selector. Conversion of both schemas is enabled by
// default. When no OptLabelPrefix matcher is present, matchers is returned
// as-is without allocating.
func extractControlMatchers(matchers []*labels.Matcher) (stripped []*labels.Matcher, convertNHCB, convertNHE, debug, matched bool, err error) {
	if !slices.ContainsFunc(matchers, IsOptMatcher) {
		return matchers, true, true, false, true, nil
	}

	var controlMatchers []*labels.Matcher
	stripped = make([]*labels.Matcher, 0, len(matchers)-1)
	for _, m := range matchers {
		switch {
		case isClassicFromMatcher(m):
			if err := validateControlMatcher(m); err != nil {
				return nil, false, false, false, false, err
			}
			controlMatchers = append(controlMatchers, m)
		case IsOptMatcher(m):
			// Strip other __opt_* matchers without affecting conversion.
		default:
			stripped = append(stripped, m)
		}
	}
	if !slices.ContainsFunc(stripped, func(m *labels.Matcher) bool { return !m.Matches("") }) {
		return nil, false, false, false, false, errOnlyControlMatchers
	}
	if len(controlMatchers) == 0 {
		return stripped, true, true, false, true, nil
	}

	matchNHCB := matchesAllControl(StoredAsNHCB, controlMatchers)
	matchNHE := matchesAllControl(StoredAsNHE, controlMatchers)
	matchNone := matchesAllControl("none", controlMatchers) || matchesAllControl("", controlMatchers)
	matchDebug := matchesAllControl("debug", controlMatchers) && hasPositiveDebugMatcher(controlMatchers)

	switch {
	case matchNHCB || matchNHE:
		return stripped, matchNHCB, matchNHE, matchDebug, true, nil
	case matchDebug:
		return stripped, true, true, true, true, nil
	case matchNone:
		return stripped, false, false, false, true, nil
	default:
		return stripped, false, false, false, false, nil
	}
}

// validateControlMatcher returns an error for equality and inequality matchers
// with unknown values. Regexp matchers are not validated, as they can
// legitimately match unknown values (e.g. =~".+").
func validateControlMatcher(m *labels.Matcher) error {
	if m.Type != labels.MatchEqual && m.Type != labels.MatchNotEqual {
		return nil
	}
	switch m.Value {
	case "", "none", StoredAsNHCB, StoredAsNHE, "debug":
		return nil
	default:
		return fmt.Errorf("%w, got %q", errInvalidControlValue, m.Value)
	}
}

func withStoredAs(lset labels.Labels, val string, b *labels.Builder) labels.Labels {
	if b == nil {
		b = labels.NewBuilder(lset)
	} else {
		b.Reset(lset)
	}
	b.Set(StoredAsLabel, val)
	return b.Labels()
}

type relabeledSeries struct {
	Series
	lset labels.Labels
}

func (s relabeledSeries) Labels() labels.Labels { return s.lset }

type storedAsSeriesSet struct {
	SeriesSet
	val     string
	builder *labels.Builder
	cur     Series
}

func newStoredAsSeriesSet(ss SeriesSet, val string) SeriesSet {
	return &storedAsSeriesSet{
		SeriesSet: ss,
		val:       val,
		builder:   labels.NewBuilder(labels.EmptyLabels()),
	}
}

func (s *storedAsSeriesSet) Next() bool {
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
		lset:   withStoredAs(ser.Labels(), s.val, s.builder),
	}
	return true
}

func (s *storedAsSeriesSet) At() Series { return s.cur }

// storedAsFilterSeriesSet filters series by StoredAsLabel matchers.
type storedAsFilterSeriesSet struct {
	SeriesSet
	matchers []*labels.Matcher
}

func (s *storedAsFilterSeriesSet) Next() bool {
	for s.SeriesSet.Next() {
		ser := s.At()
		if ser == nil {
			continue
		}
		if matchesAllControl(ser.Labels().Get(StoredAsLabel), s.matchers) {
			return true
		}
	}
	return false
}

// unifiedExponentialBounds returns the classic bounds to evaluate on all
// exponential samples of the native series in groups: the union of their
// populated bucket bounds, so that every series and sample emits the same le
// set.
func unifiedExponentialBounds(ctx context.Context, groups []histogramGroup, it chunkenc.Iterator) ([]float64, error) {
	var (
		bounds []float64
		err    error
	)
	for i := range groups {
		for _, s := range groups[i].nhcb {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if bounds, it, err = appendSeriesExponentialBounds(bounds, s, it); err != nil {
				return nil, err
			}
		}
	}
	return finalizeBounds(bounds), nil
}

// nativeSeriesKind classifies a series returned for the base metric name.
type nativeSeriesKind uint8

const (
	// nativeSeriesNone is a series that is not converted: it carries an le
	// label, has no samples, or has no non-stale sample with an enabled native
	// histogram schema.
	nativeSeriesNone nativeSeriesKind = iota
	// nativeSeriesNHCB is a series whose first enabled non-stale sample is a
	// custom buckets histogram.
	nativeSeriesNHCB
	// nativeSeriesExponential is a series whose first enabled non-stale sample
	// is an exponential schema histogram.
	nativeSeriesExponential
)

// classifyNativeSeries classifies s by its first non-stale sample that matches
// an enabled conversion schema (convertNHCB or convertNHE). Stale markers
// (histograms with a StaleNaN sum, or float StaleNaN samples) and disabled
// schemas are skipped.
func classifyNativeSeries(s Series, it chunkenc.Iterator, h *histogram.Histogram, fh *histogram.FloatHistogram, convertNHCB, convertNHE bool) (nativeSeriesKind, chunkenc.Iterator) {
	if s == nil || s.Labels().Has(labels.BucketLabel) {
		return nativeSeriesNone, it
	}
	it = s.Iterator(it)
	if it == nil {
		return nativeSeriesNone, nil
	}
	for {
		var schema int32
		switch it.Next() {
		case chunkenc.ValHistogram:
			if _, h = it.AtHistogram(h); h == nil {
				return nativeSeriesNone, it
			}
			if value.IsStaleNaN(h.Sum) {
				continue
			}
			schema = h.Schema
		case chunkenc.ValFloatHistogram:
			if _, fh = it.AtFloatHistogram(fh); fh == nil {
				return nativeSeriesNone, it
			}
			if value.IsStaleNaN(fh.Sum) {
				continue
			}
			schema = fh.Schema
		case chunkenc.ValFloat:
			if _, f := it.At(); value.IsStaleNaN(f) {
				continue
			}
			return nativeSeriesNone, it
		default:
			return nativeSeriesNone, it
		}
		switch {
		case histogram.IsCustomBucketsSchema(schema):
			if convertNHCB {
				return nativeSeriesNHCB, it
			}
		case histogram.IsExponentialSchema(schema):
			if convertNHE {
				return nativeSeriesExponential, it
			}
		default:
			return nativeSeriesNone, it
		}
	}
}

// boundsFromLeMatchers returns the finite classic upper bounds pinned by the
// given le matchers (equality or a regex that is a set of literals), sorted
// and deduplicated, or nil when the matchers do not pin a specific set of
// values. A non-nil empty result means only le="+Inf" was requested.
func boundsFromLeMatchers(leMatchers []*labels.Matcher) []float64 {
	var (
		bounds []float64
		pinned bool
	)
	for _, m := range leMatchers {
		var values []string
		switch m.Type {
		case labels.MatchEqual:
			values = []string{m.Value}
		case labels.MatchRegexp:
			values = m.SetMatches()
			if len(values) == 0 {
				// SetMatches is empty for the common le=~"0.1|0.5" spelling
				// because of the unescaped dots, so fall back to the plain
				// alternatives. Non-numeric or overly broad alternatives
				// simply pin nothing; extra bounds are harmless since
				// matchesLe still filters the output.
				values = numericAlternatives(m.Value)
				if len(values) == 0 {
					continue
				}
			}
		default:
			continue
		}
		if pinned {
			// Another matcher already pinned the values; the remaining
			// matchers are applied by matchesLe on the converted series.
			continue
		}
		pinned = true
		for _, v := range values {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				continue
			}
			bounds = append(bounds, f)
		}
	}
	if !pinned {
		return nil
	}
	return finalizeBounds(bounds)
}

// numericAlternatives splits a regex of the form "a|b|c" into its alternatives
// if every one of them is a float literal (with dots and plus signs either
// escaped or not), and returns nil otherwise.
func numericAlternatives(re string) []string {
	alternatives := strings.Split(re, "|")
	for i, alt := range alternatives {
		alt = regexFloatUnescaper.Replace(alt)
		if _, err := strconv.ParseFloat(alt, 64); err != nil {
			return nil
		}
		alternatives[i] = alt
	}
	return alternatives
}

var regexFloatUnescaper = strings.NewReplacer(`\.`, ".", `\+`, "+")

// finalizeBounds sorts bounds in place, removes duplicates and non-finite
// values, and always returns a non-nil slice.
func finalizeBounds(bounds []float64) []float64 {
	if bounds == nil {
		return []float64{}
	}
	slices.Sort(bounds)
	bounds = slices.Compact(bounds)
	out := bounds[:0]
	for _, b := range bounds {
		if !math.IsInf(b, 0) && !math.IsNaN(b) {
			out = append(out, b)
		}
	}
	return out
}

// appendSeriesExponentialBounds appends the populated bucket bounds (capped at
// exponentialAsClassicMaxSchema) of all non-stale exponential samples of s to
// dst. NHCB samples are skipped as they carry their own bounds.
func appendSeriesExponentialBounds(dst []float64, s Series, it chunkenc.Iterator) ([]float64, chunkenc.Iterator, error) {
	it = s.Iterator(it)
	if it == nil {
		return dst, nil, nil
	}
	var fh *histogram.FloatHistogram
	for {
		switch it.Next() {
		case chunkenc.ValNone:
			return dst, it, it.Err()
		case chunkenc.ValHistogram, chunkenc.ValFloatHistogram:
			// AtFloatHistogram also decodes integer histogram chunks.
			_, fh = it.AtFloatHistogram(fh)
			if fh == nil || value.IsStaleNaN(fh.Sum) || !histogram.IsExponentialSchema(fh.Schema) {
				continue
			}
			dst = histogram.AppendExponentialBounds(dst, fh, exponentialAsClassicMaxSchema)
		}
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
		if matchesLe(s.SeriesSet.At().Labels(), s.leMatchers, false) {
			return true
		}
	}
	return false
}

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

// matchesLe reports whether the le label of lset satisfies all leMatchers.
// Stored classic series are matched on the string value, as the underlying
// storage does. Converted series format le as OpenMetrics floats (e.g. "1.0"),
// while queries written against Prometheus text format classic histograms use
// "1", so with numeric set the matchers are additionally compared against the
// numeric value.
func matchesLe(lset labels.Labels, leMatchers []*labels.Matcher, numeric bool) bool {
	if len(leMatchers) == 0 {
		return true
	}
	le := lset.Get(labels.BucketLabel)
	for _, m := range leMatchers {
		if !leMatches(m, le, numeric) {
			return false
		}
	}
	return true
}

func leMatches(m *labels.Matcher, le string, numeric bool) bool {
	if !numeric {
		return m.Matches(le)
	}
	f, err := strconv.ParseFloat(le, 64)
	if err != nil {
		return m.Matches(le)
	}
	switch m.Type {
	case labels.MatchEqual, labels.MatchNotEqual:
		if want, err := strconv.ParseFloat(m.Value, 64); err == nil {
			return (want == f) == (m.Type == labels.MatchEqual)
		}
	case labels.MatchRegexp, labels.MatchNotRegexp:
		// Match the regex against both the OpenMetrics ("1.0") and the
		// Prometheus text format ("1") spelling of the bound.
		matched := m.Matches(le) == (m.Type == labels.MatchRegexp)
		if alt := strconv.FormatFloat(f, 'g', -1, 64); !matched && alt != le {
			matched = m.Matches(alt) == (m.Type == labels.MatchRegexp)
		}
		return matched == (m.Type == labels.MatchRegexp)
	}
	return m.Matches(le)
}

// nhcbToClassicSeriesSet converts NHCB series to classic histogram series
// format, resolving collisions with stored classic series per histogram group.
//
// When nhcbSet is non-nil (no stored classic series and no global bucket sort
// needed), NHCB series are streamed directly from nhcbSet one at a time. If
// exponentialMode is set and an exponential series is met while streaming,
// that series and all remaining ones are collected into groups so that their
// bounds can be unified, and conversion continues from groups.
// Otherwise, when sortSeries is false, groups are converted lazily one at a
// time as Next() advances; when sortSeries is true, all groups are converted
// on the first Next() call and sorted globally by labels.Compare.
type nhcbToClassicSeriesSet struct {
	ctx        context.Context
	firstNHCB  Series
	firstKind  nativeSeriesKind
	nhcbSet    SeriesSet
	groups     []histogramGroup
	groupIdx   int
	leMatchers []*labels.Matcher
	suffix     string
	sortSeries bool
	warnings   annotations.Annotations
	// bounds are the classic upper bounds evaluated on exponential samples
	// (pinned by le matchers or unified across all series in exponential
	// mode). When nil, bounds are discovered lazily per series the first time
	// an exponential sample is encountered.
	bounds []float64
	// exponentialMode is set for _bucket selections without pinned bounds,
	// where the streaming path has to unify bounds across the exponential
	// series it meets.
	exponentialMode bool
	convertNHCB     bool
	convertNHE      bool
	debug           bool

	initialized bool
	series      []Series
	idx         int
	err         error

	// Scratch state reused across series/groups.
	lsetBuilder  *labels.Builder
	seriesCache  histogram.ClassicSeriesCache
	builder      classicSeriesBuilder
	emitFn       func(labels.Labels, float64) error
	it           chunkenc.Iterator
	classicIt    chunkenc.Iterator
	boundsIt     chunkenc.Iterator
	h            *histogram.Histogram
	fh           *histogram.FloatHistogram
	seriesBounds []float64
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
			var (
				nhcbSeries Series
				kind       nativeSeriesKind
			)
			if s.firstNHCB != nil {
				nhcbSeries, kind = s.firstNHCB, s.firstKind
				s.firstNHCB = nil
			} else {
				for s.nhcbSet.Next() {
					cand := s.nhcbSet.At()
					if kind, s.it = classifyNativeSeries(cand, s.it, s.h, s.fh, s.convertNHCB, s.convertNHE); kind != nativeSeriesNone {
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

			if s.exponentialMode && kind == nativeSeriesExponential {
				// Bounds must be unified across all exponential series, so
				// stop streaming here: this and all remaining native series
				// are converted from groups below. Series already emitted
				// were NHCB and did not need bounds.
				if err := s.collectRemaining(nhcbSeries); err != nil {
					s.err = err
					return false
				}
				break
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

// collectRemaining switches from streaming nhcbSet to the grouped path: first
// and every remaining native series of nhcbSet become single-series groups and
// their exponential bounds are unified.
func (s *nhcbToClassicSeriesSet) collectRemaining(first Series) error {
	s.groups = append(s.groups[:0], histogramGroup{nhcb: []Series{first}})
	for s.nhcbSet.Next() {
		cand := s.nhcbSet.At()
		var kind nativeSeriesKind
		if kind, s.it = classifyNativeSeries(cand, s.it, s.h, s.fh, s.convertNHCB, s.convertNHE); kind != nativeSeriesNone {
			s.groups = append(s.groups, histogramGroup{nhcb: []Series{cand}})
		}
	}
	if err := s.nhcbSet.Err(); err != nil {
		return err
	}
	s.warnings.Merge(s.nhcbSet.Warnings())
	s.nhcbSet = nil
	s.groupIdx = 0

	var err error
	s.bounds, err = unifiedExponentialBounds(s.ctx, s.groups, s.it)
	return err
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
			if matchesLe(cs.Labels(), s.leMatchers, false) {
				if s.debug {
					cs = relabeledSeries{
						Series: cs,
						lset:   withStoredAs(cs.Labels(), StoredAsClassic, s.lsetBuilder),
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
		groupTSLoaded   bool
		filteredClassic []Series
	)
	if len(g.classic) > 0 {
		for _, cs := range g.classic {
			if matchesLe(cs.Labels(), s.leMatchers, false) {
				if s.debug {
					cs = relabeledSeries{
						Series: cs,
						lset:   withStoredAs(cs.Labels(), StoredAsClassic, s.lsetBuilder),
					}
				}
				filteredClassic = append(filteredClassic, cs)
			}
		}
	}

	// NOTE: In debug mode, stored classic and converted series get distinct
	// StoredAsLabel values, so we skip shadowing native samples at stored
	// classic timestamps to show all sources side by side.
	var loadGroupTS func() ([]int64, error)
	if !s.debug && len(g.classic) > 0 {
		loadGroupTS = func() ([]int64, error) {
			if !groupTSLoaded {
				groupTSLoaded = true
				var err error
				groupTS, s.classicIt, err = collectClassicTimestamps(g.classic, s.suffix, s.classicIt)
				if err != nil {
					return nil, err
				}
			}
			return groupTS, nil
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
		seriesFromNHCB, err := s.convertNHCBSeries(nhcbSeries, loadGroupTS, seriesDst)
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
// loadGroupTS is called lazily and at most once on the first non-skipped sample
// to load the group's classic timestamps; a nil loadGroupTS disables classic
// shadowing (used by the streaming fast path).
func (s *nhcbToClassicSeriesSet) convertNHCBSeries(nhcbSeries Series, loadGroupTS func() ([]int64, error), dst []Series) ([]Series, error) {
	baseLabels := nhcbSeries.Labels()
	nhcbLabels := baseLabels
	expLabels := baseLabels
	if s.debug {
		if s.convertNHCB {
			nhcbLabels = withStoredAs(baseLabels, StoredAsNHCB, s.lsetBuilder)
		}
		if s.convertNHE {
			expLabels = withStoredAs(baseLabels, StoredAsNHE, s.lsetBuilder)
		}
	}
	s.it = nhcbSeries.Iterator(s.it)
	if s.it == nil {
		return nil, nil
	}

	s.builder.reset()
	var (
		groupTS []int64
		tsIdx   int
	)
	bounds := s.bounds

	for {
		valType := s.it.Next()
		if valType == chunkenc.ValNone {
			break
		}

		var (
			nhcb  any
			exp   *histogram.FloatHistogram
			t     int64
			stale bool
		)

		switch valType {
		case chunkenc.ValHistogram:
			t, s.h = s.it.AtHistogram(s.h)
			if s.h == nil {
				continue
			}
			switch {
			case value.IsStaleNaN(s.h.Sum):
				stale = true
			case s.convertNHCB && histogram.IsCustomBucketsSchema(s.h.Schema):
				nhcb = s.h
			case s.convertNHE && histogram.IsExponentialSchema(s.h.Schema):
				// The classic CDF is evaluated on absolute bucket counts, so
				// integer (delta encoded) histograms go through ToFloat.
				s.fh = s.h.ToFloat(s.fh)
				exp = s.fh
			default:
				stale = true
			}
		case chunkenc.ValFloatHistogram:
			t, s.fh = s.it.AtFloatHistogram(s.fh)
			if s.fh == nil {
				continue
			}
			switch {
			case value.IsStaleNaN(s.fh.Sum):
				stale = true
			case s.convertNHCB && histogram.IsCustomBucketsSchema(s.fh.Schema):
				nhcb = s.fh
			case s.convertNHE && histogram.IsExponentialSchema(s.fh.Schema):
				exp = s.fh
			default:
				stale = true
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

		if loadGroupTS != nil {
			var err error
			groupTS, err = loadGroupTS()
			if err != nil {
				return nil, err
			}
			loadGroupTS = nil
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
		if exp != nil {
			if bounds == nil && s.suffix == histogram.ClassicSuffixBucket {
				// No series of this selector was classified as exponential by
				// its first sample, so no unified bounds exist: this series
				// switched from NHCB to an exponential schema mid-way.
				// Discover bounds from this series alone, which keeps the le
				// set stable across its samples (what rate() needs) but
				// cannot guarantee the same le set across series.
				var err error
				s.seriesBounds, s.boundsIt, err = appendSeriesExponentialBounds(s.seriesBounds[:0], nhcbSeries, s.boundsIt)
				if err != nil {
					return nil, err
				}
				s.seriesBounds = finalizeBounds(s.seriesBounds)
				bounds = s.seriesBounds
			}
			if err := histogram.ConvertExponentialToClassic(exp, bounds, expLabels, s.lsetBuilder, s.suffix, &s.seriesCache, s.emitFn); err != nil {
				return nil, err
			}
		} else if err := histogram.ConvertNHCBToClassic(nhcb, nhcbLabels, s.lsetBuilder, s.suffix, &s.seriesCache, s.emitFn); err != nil {
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
// non-stale float samples across the given classic series. For _bucket queries,
// any stored series without an le label (e.g. a non-histogram gauge/counter
// ending in _bucket) is ignored so it does not shadow converted NHCB buckets.
func collectClassicTimestamps(series []Series, suffix string, it chunkenc.Iterator) ([]int64, chunkenc.Iterator, error) {
	var ts []int64
	for _, s := range series {
		if s == nil {
			continue
		}
		if suffix == histogram.ClassicSuffixBucket && !s.Labels().Has(labels.BucketLabel) {
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
		if !matchesLe(s.labels, leMatchers, true) {
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
		if len(leMatchers) > 0 && !matchesLe(s.labels, leMatchers, true) {
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
