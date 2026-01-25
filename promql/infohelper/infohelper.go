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
	"maps"
	"slices"
	"strings"

	"github.com/grafana/regexp"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

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

// ExtractDataLabels queries info metrics and returns a map of data label names to their values.
// Data labels are all labels on the info metric except for __name__ and identifying labels.
//
// Parameters:
//   - ctx: Context for cancellation
//   - querier: Storage querier to use for fetching series
//   - infoMetricMatcher: Matcher for the info metric __name__ (e.g., MatchEqual "target_info" or MatchRegexp ".*_info")
//   - baseMetricMatchers: If provided, only info metrics matching the same identifying labels
//     as the base metrics are considered. If nil, all info metrics are returned.
//   - hints: Select hints for the storage layer
//
// Returns a map where keys are label names and values are slices of unique values for that label.
func (e *InfoLabelExtractor) ExtractDataLabels(
	ctx context.Context,
	querier storage.Querier,
	infoMetricMatcher *labels.Matcher,
	baseMetricMatchers [][]*labels.Matcher,
	hints *storage.SelectHints,
) (map[string][]string, annotations.Annotations, error) {
	var warnings annotations.Annotations

	// Build matchers for the info metric query
	infoMatchers := []*labels.Matcher{infoMetricMatcher}

	// If base metric matchers are provided, collect identifying label values from base metrics
	// and filter info metrics by those values
	if len(baseMetricMatchers) > 0 {
		idLblValues, baseWarnings, err := e.collectIdentifyingLabelValues(ctx, querier, baseMetricMatchers, hints)
		warnings.Merge(baseWarnings)
		if err != nil {
			return nil, warnings, err
		}

		// If no identifying label values found, return empty result
		if len(idLblValues) == 0 {
			return map[string][]string{}, warnings, nil
		}

		// Add regex matchers for identifying labels
		for name, vals := range idLblValues {
			infoMatchers = append(infoMatchers, labels.MustNewMatcher(labels.MatchRegexp, name, BuildRegexpAlternation(vals)))
		}
	}

	// Query info metrics
	infoSet := querier.Select(ctx, false, hints, infoMatchers...)
	warnings.Merge(infoSet.Warnings())

	// Collect data labels from info series
	dataLabels := make(map[string]map[string]struct{})

	for infoSet.Next() {
		series := infoSet.At()
		lbls := series.Labels()

		lbls.Range(func(lbl labels.Label) {
			// Skip __name__
			if lbl.Name == labels.MetricName {
				return
			}

			// Skip identifying labels
			if slices.Contains(e.config.IdentifyingLabels, lbl.Name) {
				return
			}

			// Add to data labels
			if dataLabels[lbl.Name] == nil {
				dataLabels[lbl.Name] = make(map[string]struct{})
			}
			dataLabels[lbl.Name][lbl.Value] = struct{}{}
		})
	}

	if err := infoSet.Err(); err != nil {
		return nil, warnings, err
	}

	// Convert to sorted slices
	result := make(map[string][]string, len(dataLabels))
	for name, vals := range dataLabels {
		valList := make([]string, 0, len(vals))
		for v := range vals {
			valList = append(valList, v)
		}
		slices.Sort(valList)
		result[name] = valList
	}

	return result, warnings, nil
}

// collectIdentifyingLabelValues queries base metrics and extracts values for identifying labels.
func (e *InfoLabelExtractor) collectIdentifyingLabelValues(
	ctx context.Context,
	querier storage.Querier,
	matcherSets [][]*labels.Matcher,
	hints *storage.SelectHints,
) (map[string]map[string]struct{}, annotations.Annotations, error) {
	idLblValues := make(map[string]map[string]struct{})
	var warnings annotations.Annotations

	for _, matchers := range matcherSets {
		set := querier.Select(ctx, false, hints, matchers...)
		warnings.Merge(set.Warnings())

		for set.Next() {
			series := set.At()
			lbls := series.Labels()

			// Extract identifying labels
			for _, idLbl := range e.config.IdentifyingLabels {
				val := lbls.Get(idLbl)
				if val == "" {
					continue
				}
				if idLblValues[idLbl] == nil {
					idLblValues[idLbl] = make(map[string]struct{})
				}
				idLblValues[idLbl][val] = struct{}{}
			}
		}

		if err := set.Err(); err != nil {
			return nil, warnings, err
		}
	}

	return idLblValues, warnings, nil
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
