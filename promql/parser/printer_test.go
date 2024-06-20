// Copyright 2015 The Prometheus Authors
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

package parser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestExprString(t *testing.T) {
	// A list of valid expressions that are expected to be
	// returned as out when calling String(). If out is empty the output
	// is expected to equal the input.
	inputs := []struct {
		in, out string
	}{
		{
			in: `a[^1m]`,
		},
	}

	for _, test := range inputs {
		expr, err := ParseExpr(test.in)
		require.NoError(t, err)

		exp := test.in
		if test.out != "" {
			exp = test.out
		}

		require.Equal(t, exp, expr.String())
	}
}

func TestVectorSelector_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		vs       VectorSelector
		expected string
	}{
		{
			name:     "empty value",
			vs:       VectorSelector{},
			expected: ``,
		},
		{
			name:     "no matchers with name",
			vs:       VectorSelector{Name: "foobar"},
			expected: `foobar`,
		},
		{
			name: "one matcher with name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
				},
			},
			expected: `foobar{a="x"}`,
		},
		{
			name: "two matchers with name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
					labels.MustNewMatcher(labels.MatchEqual, "b", "y"),
				},
			},
			expected: `foobar{a="x",b="y"}`,
		},
		{
			name: "two matchers without name",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
					labels.MustNewMatcher(labels.MatchEqual, "b", "y"),
				},
			},
			expected: `{a="x",b="y"}`,
		},
		{
			name: "name matcher and name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "foobar"),
				},
			},
			expected: `foobar`,
		},
		{
			name: "name matcher only",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "foobar"),
				},
			},
			expected: `{__name__="foobar"}`,
		},
		{
			name: "empty name matcher",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, ""),
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
				},
			},
			expected: `{__name__="",a="x"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.vs.String())
		})
	}
}
