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
	PlanIndexLookup(ctx context.Context, matchers []*labels.Matcher, minT, maxT int64) LookupPlan
}

// LookupPlan represents the decision of which matchers to apply during
// index lookup versus during series scanning.
type LookupPlan interface {
	// ScanMatchers returns matchers that should be applied during series scanning
	ScanMatchers() []*labels.Matcher
	// IndexMatchers returns matchers that should be applied during index lookup
	IndexMatchers() []*labels.Matcher
}

// ScanEmptyMatchersLookupPlanner implements LookupPlanner by deferring empty matchers such as l="", l=~".+" to scan matchers.
type ScanEmptyMatchersLookupPlanner struct{}

func (p *ScanEmptyMatchersLookupPlanner) PlanIndexLookup(_ context.Context, matchers []*labels.Matcher, _, _ int64) LookupPlan {
	if len(matchers) <= 1 {
		// If there is only one matcher, then using the index is usually more efficient.
		// This also covers test cases which use matchers such as {""=""} or {""=~".*"} to mean "match all series"
		return &concreteLookupPlan{
			indexMatchers: matchers,
		}
	}
	var scanMatchers, indexMatchers []*labels.Matcher

	for _, matcher := range matchers {
		if matcher.Type == labels.MatchRegexp && matcher.Value == ".*" {
			// This matches everything (empty and arbitrary values), so it doesn't reduce the selectivity of the whole set of matchers.
			continue
		}
		// Put empty string matchers and regex matchers that match everything in scan matchers.
		// These matchers are unlikely to reduce the selectivity of the matchers, but can still filter out some series, so we can't ignore them.
		if (matcher.Type == labels.MatchEqual && matcher.Value == "") ||
			(matcher.Type == labels.MatchRegexp && matcher.Value == ".+") {
			scanMatchers = append(scanMatchers, matcher)
		} else {
			// Use selective matchers in index lookup for better performance
			indexMatchers = append(indexMatchers, matcher)
		}
	}

	return &concreteLookupPlan{
		indexMatchers: indexMatchers,
		scanMatchers:  scanMatchers,
	}
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
