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

func TestOnlyIndexLookupPlanner(t *testing.T) {
	planner := &OnlyIndexLookupPlanner{}

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
		labels.MustNewMatcher(labels.MatchRegexp, "instance", ".*"),
	}

	plan := planner.PlanIndexLookup(context.TODO(), matchers, 0, 1000)

	// OnlyIndexLookupPlanner should apply all matchers during index lookup
	require.Empty(t, plan.ScanMatchers())

	require.Len(t, plan.IndexMatchers(), 2)

	// Verify the matchers are the same
	indexMatchers := plan.IndexMatchers()
	for i, matcher := range matchers {
		require.Equal(t, matcher, indexMatchers[i])
	}
}
