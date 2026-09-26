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
	"strings"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

// classicSuffixesPattern matches the suffixes of the classic histogram series
// that are assembled into a single NHCB series.
const classicSuffixesPattern = "(_bucket|_count|_sum)"

// selector is the parsed matchers of a Select call.
type selector struct {
	// matchers select the stored series.
	matchers []*labels.Matcher

	// from holds the representations to convert from. It only holds
	// representations that can be converted to what the selector selects, and
	// it is empty if nothing is converted.
	from representations
	// sourceMatchers select the series to convert from.
	sourceMatchers []*labels.Matcher
	// suffix is the suffix of the classic histogram series the selector
	// selects, e.g. _bucket. It is empty if the selector selects native
	// histograms.
	suffix string
	// leMatchers are the le matchers of a selector for classic histogram
	// series. They are applied to the converted series.
	leMatchers []*labels.Matcher
}

// newSelector parses the matchers of a Select call. convertFrom holds the
// representations to convert from.
//
// Only selectors with a single metric name matcher, which has to be an
// equality matcher, are converted:
//
//   - A name with a _bucket, _count or _sum suffix selects the classic
//     histogram series of the base name, which are converted from the native
//     histograms (NHCB and NHE) of the base name. The le matchers are applied
//     to the converted series.
//   - Any other name selects native histograms, which are converted from the
//     classic histogram series of that name. As native histograms have no le
//     label, nothing is converted if a le matcher does not match the empty
//     value.
func newSelector(matchers []*labels.Matcher, convertFrom representations) selector {
	sel := selector{matchers: matchers}

	var name *labels.Matcher
	for _, m := range matchers {
		if m.Name != model.MetricNameLabel {
			continue
		}
		if name != nil {
			return sel
		}
		name = m
	}
	if name == nil || name.Type != labels.MatchEqual {
		return sel
	}

	var le, other []*labels.Matcher
	for _, m := range matchers {
		switch m.Name {
		case model.MetricNameLabel:
		case labels.BucketLabel:
			le = append(le, m)
		default:
			other = append(other, m)
		}
	}

	if base, suffix := classicBaseName(name.Value); suffix != "" {
		sel.from = convertFrom & newRepresentations(NHCB, NHE)
		if sel.from == 0 {
			return sel
		}
		sel.suffix = suffix
		sel.leMatchers = le
		sel.sourceMatchers = append(other, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, base))
		return sel
	}

	if !convertFrom.has(Classic) {
		return sel
	}
	for _, m := range le {
		if !m.Matches("") {
			return sel
		}
	}
	// Names with invalid UTF-8 can't be matched by a regular expression.
	classicName, err := labels.NewMatcher(labels.MatchRegexp, model.MetricNameLabel, regexp.QuoteMeta(name.Value)+classicSuffixesPattern)
	if err != nil {
		return sel
	}
	sel.from = Classic.bit()
	sel.sourceMatchers = append(other, classicName)
	return sel
}

// classicBaseName returns the base name and the suffix of the name of a classic
// histogram series, e.g. foo and _bucket for foo_bucket. The suffix is empty if
// the name has no classic histogram suffix, or nothing before it.
func classicBaseName(name string) (base, suffix string) {
	for _, suffix := range []string{histogram.ClassicSuffixBucket, histogram.ClassicSuffixCount, histogram.ClassicSuffixSum} {
		if base, ok := strings.CutSuffix(name, suffix); ok && base != "" {
			return base, suffix
		}
	}
	return "", ""
}
