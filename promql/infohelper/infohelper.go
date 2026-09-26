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
	"slices"
	"strings"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

const (
	// DefaultIdentifyingLabelInstance is the standard instance identity label.
	DefaultIdentifyingLabelInstance = "instance"
	// DefaultIdentifyingLabelJob is the standard job identity label.
	DefaultIdentifyingLabelJob = "job"
)

var defaultIdentifyingLabels = [...]string{DefaultIdentifyingLabelInstance, DefaultIdentifyingLabelJob}

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

// IsDefaultIdentifyingLabel reports whether name identifies an info series.
func IsDefaultIdentifyingLabel(name string) bool {
	return name == DefaultIdentifyingLabelInstance || name == DefaultIdentifyingLabelJob
}

const (
	firstLabelIndex = iota
	secondLabelIndex
)

type identifyingLabelPresence uint8

const (
	secondLabelPresent identifyingLabelPresence = 1 << iota
	firstLabelPresent
)

var identifyingLabelPresenceBits = [...]identifyingLabelPresence{firstLabelPresent, secondLabelPresent}

type twoIdentifyingLabelGroup [2]map[string]struct{}

type twoIdentifyingLabelMatcherSetBuilder struct {
	limits      MatcherSetLimits
	groups      map[identifyingLabelPresence]*twoIdentifyingLabelGroup
	valueCount  int
	regexpBytes int
}

func newTwoIdentifyingLabelMatcherSetBuilder(limits MatcherSetLimits) twoIdentifyingLabelMatcherSetBuilder {
	return twoIdentifyingLabelMatcherSetBuilder{
		limits: limits,
		groups: map[identifyingLabelPresence]*twoIdentifyingLabelGroup{},
	}
}

func (b *twoIdentifyingLabelMatcherSetBuilder) add(metric labels.Labels, identifyingLabels [2]string) error {
	if b.groups == nil {
		b.groups = map[identifyingLabelPresence]*twoIdentifyingLabelGroup{}
	}
	values := [2]string{
		firstLabelIndex:  metric.Get(identifyingLabels[firstLabelIndex]),
		secondLabelIndex: metric.Get(identifyingLabels[secondLabelIndex]),
	}
	var presence identifyingLabelPresence
	if values[firstLabelIndex] != "" {
		presence |= firstLabelPresent
	}
	if values[secondLabelIndex] != "" {
		presence |= secondLabelPresent
	}
	if presence == 0 {
		return nil
	}

	g := b.groups[presence]
	if g == nil {
		g = &twoIdentifyingLabelGroup{}
		b.groups[presence] = g
	}
	for i, value := range values {
		if value == "" {
			continue
		}
		if g[i] == nil {
			g[i] = map[string]struct{}{}
		}
		if _, exists := g[i][value]; exists {
			continue
		}
		b.valueCount++
		if b.limits.MaxValues > 0 && b.valueCount > b.limits.MaxValues {
			return fmt.Errorf("identifying matcher values exceed limit of %d", b.limits.MaxValues)
		}
		valueBytes := escapedRegexpLen(value)
		if len(g[i]) > 0 {
			valueBytes++
		}
		b.regexpBytes += valueBytes
		if b.limits.MaxRegexpBytes > 0 && b.regexpBytes > b.limits.MaxRegexpBytes {
			return fmt.Errorf("identifying matcher regular expressions exceed limit of %d bytes", b.limits.MaxRegexpBytes)
		}
		g[i][value] = struct{}{}
	}
	return nil
}

func (b *twoIdentifyingLabelMatcherSetBuilder) matcherSets(identifyingLabels [2]string) [][]*labels.Matcher {
	presences := make([]identifyingLabelPresence, 0, len(b.groups))
	for presence := range b.groups {
		presences = append(presences, presence)
	}
	slices.Sort(presences)

	matcherSets := make([][]*labels.Matcher, 0, len(b.groups))
	for _, presence := range presences {
		g := b.groups[presence]
		matchers := make([]*labels.Matcher, 0, len(identifyingLabels))
		for i, name := range identifyingLabels {
			if presence&identifyingLabelPresenceBits[i] == 0 {
				matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, name, ""))
				continue
			}

			values := make([]string, 0, len(g[i]))
			for value := range g[i] {
				values = append(values, value)
			}
			slices.Sort(values)

			var sb strings.Builder
			for i, value := range values {
				if i > 0 {
					sb.WriteRune('|')
				}
				sb.WriteString(regexp.QuoteMeta(value))
			}
			matchers = append(matchers, labels.MustNewMatcher(labels.MatchRegexp, name, sb.String()))
		}
		matcherSets = append(matcherSets, matchers)
	}
	return matcherSets
}

// DefaultIdentifyingMatcherSetBuilder incrementally builds matcher sets for
// the standard instance and job identity labels.
type DefaultIdentifyingMatcherSetBuilder struct {
	builder twoIdentifyingLabelMatcherSetBuilder
}

// NewDefaultIdentifyingMatcherSetBuilder returns an empty builder with limits.
func NewDefaultIdentifyingMatcherSetBuilder(limits MatcherSetLimits) DefaultIdentifyingMatcherSetBuilder {
	return DefaultIdentifyingMatcherSetBuilder{builder: newTwoIdentifyingLabelMatcherSetBuilder(limits)}
}

// Add includes metric in the identifying matcher sets.
func (b *DefaultIdentifyingMatcherSetBuilder) Add(metric labels.Labels) error {
	return b.builder.add(metric, defaultIdentifyingLabels)
}

// MatcherSets returns the accumulated identifying matcher sets.
func (b *DefaultIdentifyingMatcherSetBuilder) MatcherSets() [][]*labels.Matcher {
	return b.builder.matcherSets(defaultIdentifyingLabels)
}

// IdentifyingMatcherSets builds matcher sets for every identifying-label
// presence pattern represented by metrics.
func IdentifyingMatcherSets(metrics iter.Seq[labels.Labels], identifyingLabels []string, limits MatcherSetLimits) ([][]*labels.Matcher, error) {
	if len(identifyingLabels) == 2 && identifyingLabels[0] != identifyingLabels[1] {
		return identifyingMatcherSetsForTwoLabels(metrics, identifyingLabels, limits)
	}
	return identifyingMatcherSetsGeneric(metrics, identifyingLabels, limits)
}

func identifyingMatcherSetsForTwoLabels(metrics iter.Seq[labels.Labels], identifyingLabels []string, limits MatcherSetLimits) ([][]*labels.Matcher, error) {
	labelsArray := [2]string{identifyingLabels[firstLabelIndex], identifyingLabels[secondLabelIndex]}
	builder := newTwoIdentifyingLabelMatcherSetBuilder(limits)
	var iterationErr error
	metrics(func(metric labels.Labels) bool {
		iterationErr = builder.add(metric, labelsArray)
		return iterationErr == nil
	})
	if iterationErr != nil {
		return nil, iterationErr
	}
	return builder.matcherSets(labelsArray), nil
}

func identifyingMatcherSetsGeneric(metrics iter.Seq[labels.Labels], identifyingLabels []string, limits MatcherSetLimits) ([][]*labels.Matcher, error) {
	type group map[string]map[string]struct{}
	groups := map[string]group{}
	valueCount := 0
	regexpBytes := 0

	var iterationErr error
	metrics(func(metric labels.Labels) bool {
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
			return true
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
				iterationErr = fmt.Errorf("identifying matcher values exceed limit of %d", limits.MaxValues)
				return false
			}
			valueBytes := escapedRegexpLen(value)
			if len(groups[key][name]) > 0 {
				valueBytes++
			}
			regexpBytes += valueBytes
			if limits.MaxRegexpBytes > 0 && regexpBytes > limits.MaxRegexpBytes {
				iterationErr = fmt.Errorf("identifying matcher regular expressions exceed limit of %d bytes", limits.MaxRegexpBytes)
				return false
			}
			groups[key][name][value] = struct{}{}
		}
		return true
	})
	if iterationErr != nil {
		return nil, iterationErr
	}

	matcherSets := make([][]*labels.Matcher, 0, len(groups))
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
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
	sortedValues := make([]string, 0, len(values))
	for value := range values {
		sortedValues = append(sortedValues, value)
	}
	slices.Sort(sortedValues)
	for i, v := range sortedValues {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteString(regexp.QuoteMeta(v))
	}
	return sb.String()
}
