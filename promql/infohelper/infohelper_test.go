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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/infohelper"
	"github.com/prometheus/prometheus/promql/parser"
)

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

func TestDefaultIdentifyingMatcherSetBuilder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		metrics   []labels.Labels
		expected  [][]string
		zeroValue bool
	}{
		{name: "empty", expected: [][]string{}},
		{name: "no identifiers", metrics: []labels.Labels{labels.EmptyLabels(), labels.FromStrings("job", "", "instance", "")}, expected: [][]string{}},
		{name: "single", metrics: []labels.Labels{labels.FromStrings("job", "foo")}, expected: [][]string{{`instance=""`, `job=~"foo"`}}},
		{name: "sorted and deduplicated", metrics: []labels.Labels{labels.FromStrings("job", "foo"), labels.FromStrings("job", "bar"), labels.FromStrings("job", "baz"), labels.FromStrings("job", "foo")}, expected: [][]string{{`instance=""`, `job=~"bar|baz|foo"`}}},
		{name: "escaped", metrics: []labels.Labels{labels.FromStrings("job", "c*d"), labels.FromStrings("job", "a.b")}, expected: [][]string{{`instance=""`, `job=~"a\\.b|c\\*d"`}}},
		{
			name: "presence patterns",
			metrics: []labels.Labels{
				labels.FromStrings("job", "api", "instance", "b"),
				labels.FromStrings("job", "api", "instance", "a"),
				labels.FromStrings("instance", "standalone"),
				labels.FromStrings("job", "worker"),
				labels.EmptyLabels(),
			},
			expected: [][]string{{`instance=""`, `job=~"worker"`}, {`instance=~"standalone"`, `job=""`}, {`instance=~"a|b"`, `job=~"api"`}},
		},
		{name: "zero value", zeroValue: true, metrics: []labels.Labels{labels.FromStrings("job", "api", "instance", "a")}, expected: [][]string{{`instance=~"a"`, `job=~"api"`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var builder infohelper.DefaultIdentifyingMatcherSetBuilder
			if !tc.zeroValue {
				builder = infohelper.NewDefaultIdentifyingMatcherSetBuilder(infohelper.MatcherSetLimits{})
			}
			for _, metric := range tc.metrics {
				require.NoError(t, builder.Add(metric))
			}
			actual := make([][]string, 0)
			for _, set := range builder.MatcherSets() {
				matchers := make([]string, 0, len(set))
				for _, matcher := range set {
					matchers = append(matchers, matcher.String())
				}
				actual = append(actual, matchers)
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestDefaultIdentifyingMatcherSetBuilderLimits(t *testing.T) {
	escaped := []labels.Labels{labels.FromStrings("job", "a.b"), labels.FromStrings("job", "c"), labels.FromStrings("job", "c")}
	utf8 := []labels.Labels{labels.FromStrings("job", "é"), labels.FromStrings("job", "€")}
	groups := []labels.Labels{labels.FromStrings("job", "same"), labels.FromStrings("instance", "same"), labels.FromStrings("job", "same", "instance", "same"), labels.FromStrings("job", "same", "instance", "same")}
	for _, tc := range []struct {
		name    string
		metrics []labels.Labels
		limits  infohelper.MatcherSetLimits
		err     string
	}{
		{name: "exact escaped bytes and values", metrics: escaped, limits: infohelper.MatcherSetLimits{MaxValues: 2, MaxRegexpBytes: 6}},
		{name: "too many values", metrics: escaped, limits: infohelper.MatcherSetLimits{MaxValues: 1}, err: "identifying matcher values exceed limit of 1"},
		{name: "too many escaped bytes", metrics: escaped, limits: infohelper.MatcherSetLimits{MaxRegexpBytes: 5}, err: "identifying matcher regular expressions exceed limit of 5 bytes"},
		{name: "exact UTF-8 bytes", metrics: utf8, limits: infohelper.MatcherSetLimits{MaxRegexpBytes: 6}},
		{name: "too many UTF-8 bytes", metrics: utf8, limits: infohelper.MatcherSetLimits{MaxRegexpBytes: 5}, err: "identifying matcher regular expressions exceed limit of 5 bytes"},
		{name: "exact separate group and label values", metrics: groups, limits: infohelper.MatcherSetLimits{MaxValues: 4, MaxRegexpBytes: 16}},
		{name: "too many separate group and label values", metrics: groups, limits: infohelper.MatcherSetLimits{MaxValues: 3}, err: "identifying matcher values exceed limit of 3"},
		{name: "too many separate group and label bytes", metrics: groups, limits: infohelper.MatcherSetLimits{MaxRegexpBytes: 15}, err: "identifying matcher regular expressions exceed limit of 15 bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := infohelper.NewDefaultIdentifyingMatcherSetBuilder(tc.limits)
			var err error
			for _, metric := range tc.metrics {
				if err = builder.Add(metric); err != nil {
					break
				}
			}
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, builder.MatcherSets())
		})
	}
}

func TestBuildSelectPlan(t *testing.T) {
	p := parser.NewParser(parser.Options{})
	for _, tc := range []struct {
		name              string
		expr              string
		expectedStart     int64
		expectedEnd       int64
		expectedTimestamp int64
		hasTimestamp      bool
		expectedOffset    time.Duration
	}{
		{name: "lookback", expr: "up", expectedStart: 700_001, expectedEnd: 2_000_000},
		{name: "offset", expr: "up offset 1m", expectedStart: 640_001, expectedEnd: 1_940_000, expectedOffset: time.Minute},
		{name: "timestamp", expr: "up @ 123", expectedStart: -176_999, expectedEnd: 123_000, expectedTimestamp: 123_000, hasTimestamp: true},
		{name: "same effective reference", expr: "up @ 180 offset 1m or other @ 120", expectedStart: -179_999, expectedEnd: 120_000, expectedTimestamp: 180_000, hasTimestamp: true, expectedOffset: time.Minute},
		{name: "mixed references", expr: "up @ 120 or other @ 480", expectedStart: 700_001, expectedEnd: 2_000_000},
		{name: "selector-free vector", expr: "vector(1) or up @ 120", expectedStart: 700_001, expectedEnd: 2_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := p.ParseExpr(tc.expr)
			require.NoError(t, err)
			plan := infohelper.BuildSelectPlan(expr, 1_000_000, 2_000_000, 30_000, 5*time.Minute)
			require.Equal(t, tc.expectedStart, plan.Hints.Start)
			require.Equal(t, tc.expectedEnd, plan.Hints.End)
			require.Equal(t, int64(30_000), plan.Hints.Step)
			require.Equal(t, "info", plan.Hints.Func)
			require.Equal(t, tc.expectedOffset, plan.Offset)
			if tc.hasTimestamp {
				require.NotNil(t, plan.Timestamp)
				require.Equal(t, tc.expectedTimestamp, *plan.Timestamp)
			} else {
				require.Nil(t, plan.Timestamp)
			}
		})
	}
}
