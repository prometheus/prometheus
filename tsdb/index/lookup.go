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

	"github.com/prometheus/prometheus/model/labels"
)

// LookupPlanner plans how to execute index lookups by deciding which matchers
// to apply during index lookup versus after series retrieval.
type LookupPlanner interface {
	PlanIndexLookup(ctx context.Context, plan LookupPlan, minT, maxT int64) (LookupPlan, error)
}

// ChainLookupPlanners is useful to break up the planning logic into multiple independent planners and connect them in sequence.
// The returned LookupPlanner calls the planners in sequence and feeds the output of one planner as the input of the next.
func ChainLookupPlanners(planners ...LookupPlanner) LookupPlanner {
	return chainedLookupPlanner{planners: planners}
}

type chainedLookupPlanner struct {
	planners []LookupPlanner
}

func (c chainedLookupPlanner) PlanIndexLookup(ctx context.Context, plan LookupPlan, minT, maxT int64) (LookupPlan, error) {
	var err error
	for _, p := range c.planners {
		plan, err = p.PlanIndexLookup(ctx, plan, minT, maxT)
		if err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// LookupPlan represents the decision of which matchers to apply during
// index lookup versus during series scanning.
type LookupPlan interface {
	// ScanMatchers returns matchers that should be applied during series scanning
	ScanMatchers() []*labels.Matcher
	// IndexMatchers returns matchers that should be applied during index lookup
	IndexMatchers() []*labels.Matcher
}

// NewIndexOnlyLookupPlan creates a new LookupPlan which uses the provided matchers for looking up the index only.
func NewIndexOnlyLookupPlan(matchers []*labels.Matcher) LookupPlan {
	return indexOnlyLookupPlan(matchers)
}

type indexOnlyLookupPlan []*labels.Matcher

func (l indexOnlyLookupPlan) ScanMatchers() []*labels.Matcher {
	return nil
}

func (l indexOnlyLookupPlan) IndexMatchers() []*labels.Matcher {
	return l
}

// ScanEmptyMatchersLookupPlanner implements LookupPlanner by deferring empty matchers such as l="", l=~".+" to scan matchers.
type ScanEmptyMatchersLookupPlanner struct{}

func (p *ScanEmptyMatchersLookupPlanner) PlanIndexLookup(_ context.Context, plan LookupPlan, _, _ int64) (LookupPlan, error) {
	indexMatchers := plan.IndexMatchers()
	if len(indexMatchers) <= 1 {
		// If there is only one matcher, then using the index is usually more efficient.
		// This also covers test cases which use matchers such as {""=""} or {""=~".*"} to mean "match all series"
		return plan, nil
	}
	scanMatchers := plan.ScanMatchers()

	for i := 0; i < len(indexMatchers); i++ {
		matcher := indexMatchers[i]
		if matcher.Type == labels.MatchRegexp && matcher.Value == ".*" {
			// This matches everything (empty and arbitrary values), so it doesn't reduce the selectivity of the whole set of matchers.
			continue
		}
		// Put empty string matchers and regex matchers that match everything in scan matchers.
		// These matchers are unlikely to reduce the selectivity of the matchers, but can still filter out some series, so we can't ignore them.
		if (matcher.Type == labels.MatchEqual && matcher.Value == "") ||
			(matcher.Type == labels.MatchRegexp && matcher.Value == ".+") {
			indexMatchers = append(indexMatchers[:i], indexMatchers[i+1:]...)
			i--
			scanMatchers = append(scanMatchers, matcher)
		}
	}

	if len(indexMatchers) == 0 && len(scanMatchers) > 0 {
		// Zero index matchers match no series. We retain one index matchers so that we have a base set of series which we can scan.
		indexMatchers = scanMatchers[:1]
		scanMatchers = scanMatchers[1:]
	}

	if len(scanMatchers) == 0 {
		// Avoid an allocation if the sample is simple.
		return NewIndexOnlyLookupPlan(indexMatchers), nil
	}

	return &concreteLookupPlan{
		indexMatchers: indexMatchers,
		scanMatchers:  scanMatchers,
	}, nil
}

// concreteLookupPlan implements LookupPlan by storing pre-computed index and scan matchers.
type concreteLookupPlan struct {
	indexMatchers []*labels.Matcher
	scanMatchers  []*labels.Matcher
}

func (p *concreteLookupPlan) ScanMatchers() []*labels.Matcher {
	return p.scanMatchers
}

func (p *concreteLookupPlan) IndexMatchers() []*labels.Matcher {
	return p.indexMatchers
}
