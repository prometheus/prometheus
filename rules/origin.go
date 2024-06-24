// Copyright 2023 The Prometheus Authors
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

package rules

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
)

type ruleOrigin struct{}

// RuleDetail contains information about the rule that is being evaluated.
type RuleDetail struct {
	Name   string
	Query  string
	Labels labels.Labels
	Kind   string

	// NoDependentRules is set to true if it's guaranteed that in the rule group there's no other rule
	// which depends on this one.
	NoDependentRules bool

	// NoDependencyRules is set to true if it's guaranteed that this rule doesn't depend on any other
	// rule within the rule group.
	NoDependencyRules bool
}

const (
	KindAlerting  = "alerting"
	KindRecording = "recording"
)

// NewRuleDetail creates a RuleDetail from a given Rule.
func NewRuleDetail(r Rule) RuleDetail {
	var kind string
	switch r.(type) {
	case *AlertingRule:
		kind = KindAlerting
	case *RecordingRule:
		kind = KindRecording
	default:
		panic(fmt.Sprintf(`unknown rule type "%T"`, r))
	}

	return RuleDetail{
		Name:              r.Name(),
		Query:             r.Query().String(),
		Labels:            r.Labels(),
		Kind:              kind,
		NoDependentRules:  r.NoDependentRules(),
		NoDependencyRules: r.NoDependencyRules(),
	}
}

// NewOriginContext returns a new context with data about the origin attached.
func NewOriginContext(ctx context.Context, rule RuleDetail) context.Context {
	return context.WithValue(ctx, ruleOrigin{}, rule)
}

// FromOriginContext returns the RuleDetail origin data from the context.
func FromOriginContext(ctx context.Context) RuleDetail {
	if rule, ok := ctx.Value(ruleOrigin{}).(RuleDetail); ok {
		return rule
	}
	return RuleDetail{}
}
