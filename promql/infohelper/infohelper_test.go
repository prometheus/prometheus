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

package infohelper_test

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/infohelper"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestBuildRegexpAlternation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		values   map[string]struct{}
		expected string
	}{
		{name: "empty", values: map[string]struct{}{}, expected: ""},
		{name: "single", values: map[string]struct{}{"foo": {}}, expected: "foo"},
		{name: "sorted", values: map[string]struct{}{"foo": {}, "bar": {}, "baz": {}}, expected: "bar|baz|foo"},
		{name: "escaped", values: map[string]struct{}{"a.b": {}, "c*d": {}}, expected: `a\.b|c\*d`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, infohelper.BuildRegexpAlternation(tc.values))
		})
	}
}

func TestEffectiveNameMatchers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    []*labels.Matcher
		expected []string
	}{
		{
			name:     "default",
			expected: []string{`__name__="target_info"`},
		},
		{
			name: "negative only",
			input: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "build_info"),
			},
			expected: []string{`__name__=~".+_info"`, `__name__!="build_info"`},
		},
		{
			name: "positive and negative",
			input: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+_info"),
				labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "build_info"),
			},
			expected: []string{`__name__=~".+_info"`, `__name__!="build_info"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := infohelper.EffectiveNameMatchers(tc.input)
			actualStrings := make([]string, 0, len(actual))
			for _, matcher := range actual {
				actualStrings = append(actualStrings, matcher.String())
			}
			require.Equal(t, tc.expected, actualStrings)
		})
	}
}

func TestIdentifyingMatcherSets(t *testing.T) {
	metrics := []labels.Labels{
		labels.FromStrings("job", "api", "instance", "a"),
		labels.FromStrings("job", "api", "instance", "b"),
		labels.FromStrings("job", "worker"),
		labels.FromStrings("instance", "standalone"),
		labels.EmptyLabels(),
	}

	matcherSets, err := infohelper.IdentifyingMatcherSets(slices.Values(metrics), []string{"job", "instance"}, infohelper.MatcherSetLimits{})
	require.NoError(t, err)
	actual := make([][]string, 0, len(matcherSets))
	for _, matcherSet := range matcherSets {
		matchers := make([]string, 0, len(matcherSet))
		for _, matcher := range matcherSet {
			matchers = append(matchers, matcher.String())
		}
		actual = append(actual, matchers)
	}

	require.Equal(t, [][]string{
		{`job=""`, `instance=~"standalone"`},
		{`job=~"worker"`, `instance=""`},
		{`job=~"api"`, `instance=~"a|b"`},
	}, actual)
}

func TestIdentifyingMatcherSetsLimits(t *testing.T) {
	metrics := []labels.Labels{
		labels.FromStrings("job", "a.b"),
		labels.FromStrings("job", "c"),
		labels.FromStrings("job", "c"),
	}

	matcherSets, err := infohelper.IdentifyingMatcherSets(slices.Values(metrics), []string{"job"}, infohelper.MatcherSetLimits{
		MaxValues:      2,
		MaxRegexpBytes: 6,
	})
	require.NoError(t, err)
	require.Equal(t, `job=~"a\\.b|c"`, matcherSets[0][0].String())

	_, err = infohelper.IdentifyingMatcherSets(slices.Values(metrics), []string{"job"}, infohelper.MatcherSetLimits{MaxValues: 1})
	require.EqualError(t, err, "identifying matcher values exceed limit of 1")

	_, err = infohelper.IdentifyingMatcherSets(slices.Values(metrics), []string{"job"}, infohelper.MatcherSetLimits{MaxRegexpBytes: 5})
	require.EqualError(t, err, "identifying matcher regular expressions exceed limit of 5 bytes")
}

func TestSelectHints(t *testing.T) {
	p := parser.NewParser(parser.Options{})
	for _, tc := range []struct {
		name          string
		expr          string
		expectedStart int64
		expectedEnd   int64
	}{
		{name: "lookback", expr: "up", expectedStart: 700_001, expectedEnd: 2_000_000},
		{name: "offset", expr: "up offset 1m", expectedStart: 640_001, expectedEnd: 1_940_000},
		{name: "timestamp", expr: "up @ 123", expectedStart: -176_999, expectedEnd: 123_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := p.ParseExpr(tc.expr)
			require.NoError(t, err)
			hints := infohelper.SelectHints(expr, 1_000_000, 2_000_000, 30_000, 5*time.Minute)
			require.Equal(t, tc.expectedStart, hints.Start)
			require.Equal(t, tc.expectedEnd, hints.End)
			require.Equal(t, int64(30_000), hints.Step)
			require.Equal(t, "info", hints.Func)
		})
	}
}
