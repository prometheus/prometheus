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
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

// Known limitations of the classic-to-NHCB conversion:
//
//  1. TODO: This does not support the series API (LabelNames, LabelValues, etc.).
//     Only the Select method is wrapped, so label introspection queries will not
//     reflect the converted NHCB series.
//
//  2. TODO: The results are not globally sorted. Converted series are appended
//     after the natively stored ones, in the order their classic counterparts
//     were returned by the wrapped querier.
//
//  3. TODO: The whole classic histogram is buffered in memory (one temporary
//     histogram per series and timestamp) before the first sample is returned.
//
//  4. If both a native histogram and its classic counterpart exist for the same
//     label set, both are returned. Overlapping samples then collide in PromQL
//     with "vector cannot contain metrics with the same labelset", see
//     TestClassicAsNHCBQuerier_MixedStorage for the exact cases.

// classicSuffixesPattern matches the classic histogram suffixes that are folded
// back into a single NHCB series.
const classicSuffixesPattern = "(_bucket|_count|_sum)"

// errMalformedBucketLabel is reported when the le label of a classic histogram
// bucket cannot be parsed as a float.
var errMalformedBucketLabel = errors.New("malformed bucket label")

// ClassicAsNHCBQuerier wraps a storage.Querier and converts classic histogram series
// (_bucket, _count and _sum) into Native Histograms with Custom Buckets (NHCB)
// whenever the base metric name is queried.
type ClassicAsNHCBQuerier struct {
	storage.Querier
}

// NewClassicAsNHCBQuerier returns a new querier that wraps the given querier
// and converts classic histograms to NHCB for queries.
func NewClassicAsNHCBQuerier(q storage.Querier) storage.Querier {
	return &ClassicAsNHCBQuerier{Querier: q}
}

// ClassicAsNHCBStorage wraps a storage.Storage and applies classic-to-NHCB conversion
// to queriers.
type ClassicAsNHCBStorage struct {
	storage.Storage
}

// NewClassicAsNHCBStorage returns a new storage that wraps the given storage
// and applies classic-to-NHCB conversion to queriers.
func NewClassicAsNHCBStorage(s storage.Storage) storage.Storage {
	return &ClassicAsNHCBStorage{Storage: s}
}

// Querier implements the storage.Storage interface.
func (s *ClassicAsNHCBStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return NewClassicAsNHCBQuerier(q), nil
}

// Select implements the storage.Querier interface.
func (q *ClassicAsNHCBQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	nameMatcher, baseMatchers := extractMetricNameMatcher(matchers)
	classicNameMatcher := newClassicSuffixMatcher(nameMatcher)
	if classicNameMatcher == nil {
		// Not a query we can expand (no usable __name__ matcher, or the name
		// already carries a classic histogram suffix), pass through.
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}
	for _, m := range baseMatchers {
		if m.Name == labels.BucketLabel {
			// NHCB series never carry a le label, so such a query cannot be
			// about a native histogram. Pass through.
			return q.Querier.Select(ctx, sortSeries, hints, matchers...)
		}
	}

	nativeSet := q.Querier.Select(ctx, sortSeries, hints, matchers...)
	if nativeSet.Err() != nil {
		return nativeSet
	}

	var nativeSeries []storage.Series
	for nativeSet.Next() {
		nativeSeries = append(nativeSeries, nativeSet.At())
	}
	if err := nativeSet.Err(); err != nil {
		return storage.ErrSeriesSet(err)
	}

	seriesSets := make([]storage.SeriesSet, 0, 2)
	if len(nativeSeries) > 0 {
		seriesSets = append(seriesSets, &bufferedSeriesSet{series: nativeSeries, warnings: nativeSet.Warnings()})
	}

	classicMatchers := make([]*labels.Matcher, 0, len(baseMatchers)+1)
	classicMatchers = append(classicMatchers, baseMatchers...)
	classicMatchers = append(classicMatchers, classicNameMatcher)

	classicSet := q.Querier.Select(ctx, sortSeries, hints, classicMatchers...)
	if classicSet.Err() != nil {
		return classicSet
	}
	seriesSets = append(seriesSets, &classicToNHCBSeriesSet{classicSet: classicSet})

	return &multipleSeriesSet{seriesSet: seriesSets}
}

// extractMetricNameMatcher separates the __name__ matcher from the other
// matchers. The returned __name__ matcher is nil if there is none.
func extractMetricNameMatcher(matchers []*labels.Matcher) (*labels.Matcher, []*labels.Matcher) {
	var (
		nameMatcher  *labels.Matcher
		baseMatchers = make([]*labels.Matcher, 0, len(matchers))
	)
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel && nameMatcher == nil {
			nameMatcher = m
			continue
		}
		baseMatchers = append(baseMatchers, m)
	}
	return nameMatcher, baseMatchers
}

// newClassicSuffixMatcher turns a __name__ matcher for a native histogram into
// a matcher selecting the classic histogram series of the same metric. It
// returns nil if no such matcher can be derived, which is the case when there
// is no __name__ matcher, when the matcher is negative (the set of matching
// names is unbounded) or when the name already has a classic histogram suffix.
func newClassicSuffixMatcher(m *labels.Matcher) *labels.Matcher {
	if m == nil || histogramSuffix(m.Value) != "" {
		return nil
	}
	var value string
	switch m.Type {
	case labels.MatchEqual:
		value = regexp.QuoteMeta(m.Value) + classicSuffixesPattern
	case labels.MatchRegexp:
		// Matchers are fully anchored, so the alternation of the original
		// expression has to be grouped before appending the suffixes.
		value = "(?:" + m.Value + ")" + classicSuffixesPattern
	default:
		return nil
	}
	classicMatcher, err := labels.NewMatcher(labels.MatchRegexp, model.MetricNameLabel, value)
	if err != nil {
		return nil
	}
	return classicMatcher
}

// nhcbGroup collects the classic histogram series of one metric, keyed by
// sample timestamp, so they can be converted into NHCB samples.
type nhcbGroup struct {
	labels     labels.Labels
	name       string
	histograms map[int64]*convertnhcb.TempHistogram
	// stale holds the timestamps of the stale markers of the classic series.
	stale map[int64]struct{}
}

// classicToNHCBSeriesSet converts classic histogram series into NHCB series.
type classicToNHCBSeriesSet struct {
	classicSet storage.SeriesSet

	series   []storage.Series
	idx      int
	err      error
	warnings annotations.Annotations
}

func (s *classicToNHCBSeriesSet) Next() bool {
	if s.err != nil {
		return false
	}
	// Convert all classic series on the first Next() call. A single NHCB series
	// is assembled from multiple classic series, so nothing can be returned
	// before the whole set has been consumed.
	if s.series == nil && !s.convert() {
		return false
	}
	if s.idx < len(s.series) {
		s.idx++
		return true
	}
	return false
}

// convert drains the wrapped series set and builds the NHCB series. It reports
// whether it succeeded.
func (s *classicToNHCBSeriesSet) convert() bool {
	s.series = make([]storage.Series, 0)

	var (
		groups  []*nhcbGroup
		byHash  = make(map[uint64][]int)
		chkIter chunkenc.Iterator
	)
	for s.classicSet.Next() {
		series := s.classicSet.At()
		if series == nil {
			continue
		}
		lset := series.Labels()
		suffixType, baseName := convertnhcb.GetHistogramMetricBaseName(lset.Get(model.MetricNameLabel))
		if suffixType == convertnhcb.SuffixNone {
			continue
		}
		var le float64
		if suffixType == convertnhcb.SuffixBucket {
			bucket := lset.Get(labels.BucketLabel)
			var err error
			le, err = strconv.ParseFloat(bucket, 64)
			if err != nil || math.IsNaN(le) {
				s.warnings.Add(annotations.NewClassicToNHCBConversionWarning(baseName, fmt.Errorf("%w %q", errMalformedBucketLabel, bucket)))
				continue
			}
		}

		baseLabels := convertnhcb.GetHistogramMetricBase(lset, baseName)
		group := lookupOrCreateGroup(&groups, byHash, baseLabels, baseName)

		chkIter = series.Iterator(chkIter)
		for {
			valType := chkIter.Next()
			if valType == chunkenc.ValNone {
				break
			}
			if valType != chunkenc.ValFloat {
				// Classic histogram series only hold float samples.
				continue
			}
			t, v := chkIter.At()
			if value.IsStaleNaN(v) {
				// Stale markers are not part of the classic histogram at t.
				// If all its series are stale at t, the NHCB is marked stale
				// below, e.g. because the target went away. Otherwise the
				// remaining series are converted, e.g. because the bucket
				// layout changed.
				group.stale[t] = struct{}{}
				continue
			}
			temp, ok := group.histograms[t]
			if !ok {
				h := convertnhcb.NewTempHistogram()
				temp = &h
				group.histograms[t] = temp
			}
			switch suffixType {
			case convertnhcb.SuffixBucket:
				_ = temp.SetBucketCount(le, v)
			case convertnhcb.SuffixCount:
				_ = temp.SetCount(v)
			case convertnhcb.SuffixSum:
				_ = temp.SetSum(v)
			}
		}
		if err := chkIter.Err(); err != nil {
			s.err = err
			return false
		}
	}
	if err := s.classicSet.Err(); err != nil {
		s.err = err
		return false
	}

	for _, group := range groups {
		timestamps := make([]int64, 0, len(group.histograms)+len(group.stale))
		for t := range group.histograms {
			timestamps = append(timestamps, t)
		}
		for t := range group.stale {
			if _, ok := group.histograms[t]; !ok {
				timestamps = append(timestamps, t)
			}
		}
		slices.Sort(timestamps)

		samples := make([]chunks.Sample, 0, len(timestamps))
		for _, t := range timestamps {
			temp, ok := group.histograms[t]
			if !ok {
				// All the classic series of the histogram are stale at t.
				samples = append(samples, hSample{t: t, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
				continue
			}
			h, fh, err := temp.Convert()
			if err != nil {
				// A classic histogram that cannot be converted (e.g. a
				// non-cumulative or incomplete exposition) is skipped, the rest
				// of the series is still returned.
				s.warnings.Add(annotations.NewClassicToNHCBConversionWarning(group.name, err))
				continue
			}
			switch {
			case h != nil:
				samples = append(samples, hSample{t: t, h: h})
			case fh != nil:
				samples = append(samples, fhSample{t: t, fh: fh})
			}
		}
		if len(samples) == 0 {
			continue
		}
		s.series = append(s.series, storage.NewListSeries(group.labels, samples))
	}
	return true
}

// lookupOrCreateGroup returns the group for lset, creating it if needed. Groups
// are indexed by label hash, the slice keeps the insertion order stable.
func lookupOrCreateGroup(groups *[]*nhcbGroup, byHash map[uint64][]int, lset labels.Labels, name string) *nhcbGroup {
	h := lset.Hash()
	for _, idx := range byHash[h] {
		if labels.Equal((*groups)[idx].labels, lset) {
			return (*groups)[idx]
		}
	}
	group := &nhcbGroup{
		labels:     lset,
		name:       name,
		histograms: make(map[int64]*convertnhcb.TempHistogram),
		stale:      make(map[int64]struct{}),
	}
	byHash[h] = append(byHash[h], len(*groups))
	*groups = append(*groups, group)
	return group
}

func (s *classicToNHCBSeriesSet) At() storage.Series {
	if s.idx == 0 || s.idx > len(s.series) {
		return nil
	}
	return s.series[s.idx-1]
}

func (s *classicToNHCBSeriesSet) Err() error {
	return s.err
}

func (s *classicToNHCBSeriesSet) Warnings() annotations.Annotations {
	var w annotations.Annotations
	w.Merge(s.classicSet.Warnings())
	w.Merge(s.warnings)
	return w
}
