// Copyright 2024 The Prometheus Authors
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

package index

import (
	"github.com/prometheus/prometheus/model/labels"
)

// PostingsForLabelMatchingTestSeries is an ordered slice of label sets that PostingsForLabelMatchingTestCases are based on.
var PostingsForLabelMatchingTestSeries = []labels.Labels{
	labels.FromStrings("i", "a", "n", "1"),
	labels.FromStrings("i", "b", "n", "1"),
	labels.FromStrings("n", "1"),
	labels.FromStrings("n", "2"),
	labels.FromStrings("n", "2.5"),
}

// PostingsForLabelMatchingTestCase is a PostingsForLabelMatching test case.
type PostingsForLabelMatchingTestCase struct {
	Matcher *labels.Matcher
	Exp     []labels.Labels
}

// PostingsForLabelMatchingTestCases is a slice of PostingsForLabelMatchingTestCases,
// that are common to PostingsForLabelMatching and PostingsForMatchers tests.
var PostingsForLabelMatchingTestCases = []PostingsForLabelMatchingTestCase{
	// Simple equals.
	{
		Matcher: labels.MustNewMatcher(labels.MatchEqual, "n", "1"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1"),
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
		},
	},
	// Not equals.
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotEqual, "n", "1"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
			labels.FromStrings("n", "2.5"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotEqual, "i", ""),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotEqual, "missing", ""),
		Exp:     []labels.Labels{},
	},
	// Regexp.
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "n", "^1$"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1"),
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
		},
	},
	// Not regexp.
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotRegexp, "n", "^1$"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
			labels.FromStrings("n", "2.5"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotRegexp, "n", "1"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
			labels.FromStrings("n", "2.5"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotRegexp, "n", "1|2.5"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchNotRegexp, "n", "(1|2.5)"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
		},
	},
	// Set optimization for regexp.
	// Refer to https://github.com/prometheus/prometheus/issues/2651.
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "n", "1|2"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1"),
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
			labels.FromStrings("n", "2"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "i", "a|b"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "i", "(a|b)"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "1", "i", "a"),
			labels.FromStrings("n", "1", "i", "b"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "n", "x1|2"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
		},
	},
	{
		Matcher: labels.MustNewMatcher(labels.MatchRegexp, "n", "2|2\\.5"),
		Exp: []labels.Labels{
			labels.FromStrings("n", "2"),
			labels.FromStrings("n", "2.5"),
		},
	},
}
