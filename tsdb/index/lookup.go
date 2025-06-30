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

// OnlyIndexLookupPlanner implements LookupPlanner by always applying all matchers
// during index lookup. This maintains current behavior and serves as the default fallback.
type OnlyIndexLookupPlanner struct{}

func (p *OnlyIndexLookupPlanner) PlanIndexLookup(_ context.Context, matchers []*labels.Matcher, _, _ int64) LookupPlan {
	return &indexOnlyLookupPlan{matchers: matchers}
}

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

// indexOnlyLookupPlan implements LookupPlan by applying all matchers during index lookup.
// This maintains the current behavior and serves as the default fallback.
type indexOnlyLookupPlan struct {
	matchers []*labels.Matcher
}

func (p *indexOnlyLookupPlan) ScanMatchers() []*labels.Matcher {
	return nil
}

func (p *indexOnlyLookupPlan) IndexMatchers() []*labels.Matcher {
	return p.matchers
}
