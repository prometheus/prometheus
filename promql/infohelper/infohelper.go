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

// Package infohelper provides shared matching helpers for PromQL info metrics.
package infohelper

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// DefaultIdentifyingLabels are the standard labels used to match base metrics to info metrics.
var DefaultIdentifyingLabels = []string{"instance", "job"}

// DefaultInfoMetricName is the default info metric name when none is specified.
const DefaultInfoMetricName = "target_info"

// EffectiveNameMatchers returns the metric-name matchers used to select info
// series. Negative-only selections are restricted to info metric names.
func EffectiveNameMatchers(matchers []*labels.Matcher) []*labels.Matcher {
	for _, m := range matchers {
		if m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp {
			return matchers
		}
	}
	if len(matchers) > 0 {
		return append([]*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_info")}, matchers...)
	}

	return []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, DefaultInfoMetricName)}
}

// MatchesAll reports whether value matches every matcher.
func MatchesAll(value string, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(value) {
			return false
		}
	}
	return true
}

// MatcherSetLimits bounds the intermediate values and regular expressions used
// to construct identifying matcher sets. Zero disables the corresponding bound.
type MatcherSetLimits struct {
	MaxValues      int
	MaxRegexpBytes int
}

// IdentifyingMatcherSets builds matcher sets for every identifying-label
// presence pattern represented by metrics.
func IdentifyingMatcherSets(metrics iter.Seq[labels.Labels], identifyingLabels []string, limits MatcherSetLimits) ([][]*labels.Matcher, error) {
	type group map[string]map[string]struct{}
	groups := map[string]group{}
	valueCount := 0
	regexpBytes := 0

	for metric := range metrics {
		presence := make([]byte, len(identifyingLabels))
		values := make(map[string]string, len(identifyingLabels))
		hasIdentifier := false
		for i, name := range identifyingLabels {
			value := metric.Get(name)
			if value == "" {
				presence[i] = '0'
				continue
			}
			presence[i] = '1'
			values[name] = value
			hasIdentifier = true
		}
		if !hasIdentifier {
			continue
		}

		key := string(presence)
		if groups[key] == nil {
			groups[key] = group{}
		}
		for name, value := range values {
			if groups[key][name] == nil {
				groups[key][name] = map[string]struct{}{}
			}
			if _, exists := groups[key][name][value]; exists {
				continue
			}
			valueCount++
			if limits.MaxValues > 0 && valueCount > limits.MaxValues {
				return nil, fmt.Errorf("identifying matcher values exceed limit of %d", limits.MaxValues)
			}
			valueBytes := escapedRegexpLen(value)
			if len(groups[key][name]) > 0 {
				valueBytes++
			}
			regexpBytes += valueBytes
			if limits.MaxRegexpBytes > 0 && regexpBytes > limits.MaxRegexpBytes {
				return nil, fmt.Errorf("identifying matcher regular expressions exceed limit of %d bytes", limits.MaxRegexpBytes)
			}
			groups[key][name][value] = struct{}{}
		}
	}

	matcherSets := make([][]*labels.Matcher, 0, len(groups))
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		matchers := make([]*labels.Matcher, 0, len(identifyingLabels))
		for i, name := range identifyingLabels {
			if key[i] == '0' {
				matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, name, ""))
				continue
			}
			matchers = append(matchers, labels.MustNewMatcher(labels.MatchRegexp, name, BuildRegexpAlternation(groups[key][name])))
		}
		matcherSets = append(matcherSets, matchers)
	}
	return matcherSets, nil
}

func escapedRegexpLen(value string) int {
	length := len(value)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\', '.', '+', '*', '?', '(', ')', '|', '[', ']', '{', '}', '^', '$':
			length++
		}
	}
	return length
}

// SelectPlan contains the storage bounds and evaluation reference for info series.
type SelectPlan struct {
	Hints     storage.SelectHints
	Timestamp *int64
	Offset    time.Duration
}

type seriesReference struct {
	timestamp *int64
	offset    time.Duration
}

func (r seriesReference) equal(other seriesReference) bool {
	if r.timestamp == nil || other.timestamp == nil {
		return r.timestamp == nil && other.timestamp == nil && r.offset.Milliseconds() == other.offset.Milliseconds()
	}
	return *r.timestamp-r.offset.Milliseconds() == *other.timestamp-other.offset.Milliseconds()
}

// SelectTimestampAndOffset derives a shared reference for vector-producing paths.
func SelectTimestampAndOffset(expr parser.Expr) (nodeTimestamp *int64, offset time.Duration, uniform bool) {
	var (
		first         seriesReference
		found         bool
		referenceFree bool
	)
	uniform = true

	var inspect func(parser.Expr, []parser.Node) bool
	inspect = func(expr parser.Expr, path []parser.Node) bool {
		if expr.Type() != parser.ValueTypeVector && expr.Type() != parser.ValueTypeMatrix {
			return false
		}

		if n, ok := expr.(*parser.VectorSelector); ok {
			ref := seriesReference{timestamp: n.Timestamp, offset: n.OriginalOffset}
			// Enclosing subqueries shift the reference until an @ timestamp anchors it.
			for i := len(path) - 1; ref.timestamp == nil && i >= 0; i-- {
				if sq, ok := path[i].(*parser.SubqueryExpr); ok {
					ref.offset += sq.OriginalOffset
					ref.timestamp = sq.Timestamp
				}
			}

			if !found {
				first = ref
				found = true
			} else if !first.equal(ref) {
				uniform = false
			}
			return true
		}

		path = append(path, expr)
		if call, ok := expr.(*parser.Call); ok && call.Func.Name == "info" {
			// The second argument is selector syntax, not an evaluated vector.
			return inspect(call.Args[0], path)
		}

		hasSelector := false
		for child := range parser.ChildrenIter(expr) {
			childExpr, ok := child.(parser.Expr)
			if ok && inspect(childExpr, path) {
				hasSelector = true
			}
		}
		if !hasSelector {
			// Selector-free vectors use the evaluator time.
			referenceFree = true
		}
		return hasSelector
	}

	inspect(expr, nil)
	if !found {
		return nil, 0, false
	}
	return first.timestamp, first.offset, uniform && !referenceFree
}

// BuildSelectPlan derives the shared info-series reference for expr.
func BuildSelectPlan(expr parser.Expr, start, end, step int64, lookbackDelta time.Duration) SelectPlan {
	nodeTimestamp, offset, uniform := SelectTimestampAndOffset(expr)
	if !uniform {
		nodeTimestamp = nil
		offset = 0
	}

	if nodeTimestamp != nil {
		start = *nodeTimestamp
		end = *nodeTimestamp
	}
	start -= lookbackDelta.Milliseconds() - 1
	start -= offset.Milliseconds()
	end -= offset.Milliseconds()

	return SelectPlan{
		Hints: storage.SelectHints{
			Start: start,
			End:   end,
			Step:  step,
			Func:  "info",
		},
		Timestamp: nodeTimestamp,
		Offset:    offset,
	}
}

// BuildRegexpAlternation returns a deterministic, escaped alternation for the provided values.
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
