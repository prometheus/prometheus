// Copyright 2025 The Prometheus Authors
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
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestScanEmptyMatchersLookupPlanner(t *testing.T) {
	planner := &ScanEmptyMatchersLookupPlanner{}

	testCases := []struct {
		name          string
		matchers      []*labels.Matcher
		expectedIndex []*labels.Matcher
		expectedScan  []*labels.Matcher
	}{
		{
			name: "splits matchers between index and scan",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"), // index
				labels.MustNewMatcher(labels.MatchRegexp, "instance", ".*"),   // dropped
				labels.MustNewMatcher(labels.MatchEqual, "env", ""),           // scan
				labels.MustNewMatcher(labels.MatchNotEqual, "status", "down"), // index
				labels.MustNewMatcher(labels.MatchRegexp, "region", ".+"),     // scan
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
				labels.MustNewMatcher(labels.MatchNotEqual, "status", "down"),
			},
			expectedScan: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "env", ""),
				labels.MustNewMatcher(labels.MatchRegexp, "region", ".+"),
			},
		},
		{
			name: "all selective matchers go to index",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
				labels.MustNewMatcher(labels.MatchNotEqual, "instance", "localhost"),
				labels.MustNewMatcher(labels.MatchRegexp, "env", "prod|staging"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
				labels.MustNewMatcher(labels.MatchNotEqual, "instance", "localhost"),
				labels.MustNewMatcher(labels.MatchRegexp, "env", "prod|staging"),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name: "even with only non-selective matchers, there is still one which stays as index matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
				labels.MustNewMatcher(labels.MatchRegexp, "instance", ".*"),
				labels.MustNewMatcher(labels.MatchRegexp, "env", ".+"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
			},
			expectedScan: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "env", ".+"),
			},
		},
		{
			name: "all non-selective matchers go to scan",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
				labels.MustNewMatcher(labels.MatchRegexp, "instance", ".*"),
				labels.MustNewMatcher(labels.MatchRegexp, "env", ".+"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			expectedScan: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
				labels.MustNewMatcher(labels.MatchRegexp, "env", ".+"),
			},
		},
		{
			name: "single matcher goes to index",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name: "single empty matcher goes to index",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", ""),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name: "single match-all regex goes to index",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", ".*"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", ".*"),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name: "single match-non-empty regex goes to index",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", ".+"),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", ".+"),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name: "single empty matcher goes to index; this is an invalid matcher on the API, but some internal code uses it",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "", ""),
			},
			expectedIndex: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "", ""),
			},
			expectedScan: []*labels.Matcher{},
		},
		{
			name:          "empty matchers slice",
			matchers:      []*labels.Matcher{},
			expectedIndex: []*labels.Matcher{},
			expectedScan:  []*labels.Matcher{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan, _ := planner.PlanIndexLookup(context.TODO(), NewIndexOnlyLookupPlan(tc.matchers), 0, 1000)

			require.Equal(t, matchersToString(tc.expectedScan), matchersToString(plan.ScanMatchers()))
			require.Equal(t, matchersToString(tc.expectedIndex), matchersToString(plan.IndexMatchers()))
		})
	}
}

func matchersToString(matchers []*labels.Matcher) string {
	if len(matchers) == 0 {
		return ""
	}
	var result string
	for i, m := range matchers {
		if i > 0 {
			result += ","
		}
		result += m.String()
	}
	return result
}
