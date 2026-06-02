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

// Package infohelper provides utilities for extracting data labels from info metrics.
// Info metrics (like target_info) contain metadata labels that can be used to enrich
// other time series via identifying labels like "job" and "instance".
package infohelper

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/grafana/regexp"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// InfoLabelRecord is one data label discovered on an info metric, with the
// unique values seen across matching series and the relevance score produced
// by the optional filter (1.0 when the filter accepted the name unconditionally
// or no filter was supplied).
type InfoLabelRecord struct {
	Name   string
	Values []string
	Score  float64
}

// DefaultIdentifyingLabels are the standard labels used to match base metrics to info metrics.
// These are the default identifying labels for Prometheus info metrics.
var DefaultIdentifyingLabels = []string{"instance", "job"}

// DefaultInfoMetricName is the default info metric name when none is specified.
const DefaultInfoMetricName = "target_info"

// Config controls behavior of info label extraction.
type Config struct {
	// IdentifyingLabels are the labels used to match base metrics to info metrics.
	// These labels are excluded from the data labels returned.
	IdentifyingLabels []string

	// DefaultInfoMetric is the default info metric to query when none is specified.
	DefaultInfoMetric string
}

// DefaultConfig returns the standard config for Prometheus info metrics.
func DefaultConfig() Config {
	return Config{
		IdentifyingLabels: DefaultIdentifyingLabels,
		DefaultInfoMetric: DefaultInfoMetricName,
	}
}

// InfoLabelExtractor extracts data labels from info metrics.
type InfoLabelExtractor struct {
	config Config
}

// New creates an InfoLabelExtractor with the given config.
func New(config Config) *InfoLabelExtractor {
	return &InfoLabelExtractor{config: config}
}

// NewWithDefaults creates an InfoLabelExtractor with default config.
func NewWithDefaults() *InfoLabelExtractor {
	return New(DefaultConfig())
}

// ExtractDataLabels queries info metrics and returns the discovered data
// labels as records. Data labels are all labels on an info metric except
// __name__ and the configured identifying labels.
//
// Parameters:
//   - ctx: cancellation context.
//   - querier: storage querier used to fetch series.
//   - infoMetricMatcher: matcher for the info metric __name__
//     (e.g. MatchEqual "target_info" or MatchRegexp ".*_info").
//   - identifyingLabelValues: if non-empty, restricts info metrics to those
//     whose identifying labels (keys) take any of the provided values.
//   - hints: select hints forwarded to the storage layer.
//   - filter: optional storage.Filter applied to label NAMES. When nil, all
//     non-identifying labels are accepted with score 1.0. When non-nil, only
//     accepted names are returned and Score reflects the filter's relevance.
//   - namesLimit: if > 0, caps the number of distinct label NAMES collected.
//     Once the cap is reached, the extractor still drains the series set to
//     accumulate values for names already accepted, but rejects any
//     not-yet-seen names. A warning is added to the returned annotations.
//     Note that when names are truncated, callers using a score-based sort
//     may miss high-scoring names that arrived after the cap.
//   - valuesLimit: if > 0, truncates the per-label values slice to this length.
//     Truncation happens after alphabetical sort.
//
// Records are returned in undefined order; callers apply their own sort.
func (e *InfoLabelExtractor) ExtractDataLabels(
	ctx context.Context,
	querier storage.Querier,
	infoMetricMatcher *labels.Matcher,
	identifyingLabelValues map[string]map[string]struct{},
	hints *storage.SelectHints,
	filter storage.Filter,
	namesLimit, valuesLimit int,
) ([]InfoLabelRecord, annotations.Annotations, error) {
	var warnings annotations.Annotations

	infoMatchers := []*labels.Matcher{infoMetricMatcher}
	if len(identifyingLabelValues) > 0 {
		for name, vals := range identifyingLabelValues {
			infoMatchers = append(infoMatchers, labels.MustNewMatcher(labels.MatchRegexp, name, BuildRegexpAlternation(vals)))
		}
	}

	infoSet := querier.Select(ctx, false, hints, infoMatchers...)
	warnings.Merge(infoSet.Warnings())

	type dataLabelEntry struct {
		values map[string]struct{}
		score  float64
	}
	// nameDecision memoises filter outcomes (and the score) per label name so
	// the per-series Range below doesn't re-invoke the filter for every series
	// that exposes the same label.
	nameDecision := map[string]struct {
		accept bool
		score  float64
	}{}
	dataLabels := map[string]*dataLabelEntry{}
	namesTruncated := false

	for infoSet.Next() {
		if ctx.Err() != nil {
			return nil, warnings, ctx.Err()
		}

		infoSet.At().Labels().Range(func(lbl labels.Label) {
			if lbl.Name == labels.MetricName {
				return
			}
			if slices.Contains(e.config.IdentifyingLabels, lbl.Name) {
				return
			}
			decision, seen := nameDecision[lbl.Name]
			if !seen {
				if filter == nil {
					decision.accept = true
					decision.score = 1.0
				} else {
					decision.accept, decision.score = filter.Accept(lbl.Name)
				}
				nameDecision[lbl.Name] = decision
			}
			if !decision.accept {
				return
			}
			entry := dataLabels[lbl.Name]
			if entry == nil {
				// Enforce the names cap: drop new names once at the cap,
				// but keep accepting values for already-collected names.
				if namesLimit > 0 && len(dataLabels) >= namesLimit {
					namesTruncated = true
					return
				}
				entry = &dataLabelEntry{values: map[string]struct{}{}, score: decision.score}
				dataLabels[lbl.Name] = entry
			}
			entry.values[lbl.Value] = struct{}{}
		})
	}

	if err := infoSet.Err(); err != nil {
		return nil, warnings, err
	}

	if namesTruncated {
		warnings = warnings.Add(fmt.Errorf("info-labels names truncated at %d; narrow metric_match or raise --web.search.max-limit", namesLimit))
	}

	records := make([]InfoLabelRecord, 0, len(dataLabels))
	for name, entry := range dataLabels {
		vals := make([]string, 0, len(entry.values))
		for v := range entry.values {
			vals = append(vals, v)
		}
		slices.Sort(vals)
		if valuesLimit > 0 && len(vals) > valuesLimit {
			vals = vals[:valuesLimit]
		}
		records = append(records, InfoLabelRecord{Name: name, Values: vals, Score: entry.score})
	}

	return records, warnings, nil
}

// IdentifyingLabels returns the identifying labels configured for this extractor.
func (e *InfoLabelExtractor) IdentifyingLabels() []string {
	return e.config.IdentifyingLabels
}

// DefaultInfoMetric returns the default info metric configured for this extractor.
func (e *InfoLabelExtractor) DefaultInfoMetric() string {
	return e.config.DefaultInfoMetric
}

// BuildRegexpAlternation creates a regex pattern that matches any of the provided values.
// Values are escaped for use in regular expressions and joined with the '|' alternation operator.
// The values are sorted to ensure deterministic output.
// For example: {"foo": {}, "bar": {}, "baz": {}} -> "bar|baz|foo"
// Special regex characters in values are escaped: {"a.b": {}, "c*d": {}} -> "a\\.b|c\\*d"
//
// This is used to build efficient regex matchers for filtering info metrics by
// identifying label values extracted from base metrics.
func BuildRegexpAlternation(values map[string]struct{}) string {
	if len(values) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, v := range slices.Sorted(maps.Keys(values)) {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteString(regexp.QuoteMeta(v))
	}
	return sb.String()
}
