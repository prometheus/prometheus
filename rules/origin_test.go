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
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

type unknownRule struct{}

func (unknownRule) Name() string          { return "" }
func (unknownRule) Labels() labels.Labels { return labels.EmptyLabels() }
func (unknownRule) Eval(context.Context, time.Duration, time.Time, QueryFunc, *url.URL, int) (promql.Vector, error) {
	return nil, nil
}
func (unknownRule) String() string                       { return "" }
func (unknownRule) Query() parser.Expr                   { return nil }
func (unknownRule) SetLastError(error)                   {}
func (unknownRule) LastError() error                     { return nil }
func (unknownRule) SetHealth(RuleHealth)                 {}
func (unknownRule) Health() RuleHealth                   { return "" }
func (unknownRule) SetEvaluationDuration(time.Duration)  {}
func (unknownRule) GetEvaluationDuration() time.Duration { return 0 }
func (unknownRule) SetEvaluationTimestamp(time.Time)     {}
func (unknownRule) GetEvaluationTimestamp() time.Time    { return time.Time{} }
func (unknownRule) SetDependentRules([]Rule)             {}
func (unknownRule) NoDependentRules() bool               { return false }
func (unknownRule) DependentRules() []Rule               { return nil }
func (unknownRule) SetDependencyRules([]Rule)            {}
func (unknownRule) NoDependencyRules() bool              { return false }
func (unknownRule) DependencyRules() []Rule              { return nil }

func TestNewRuleDetailPanics(t *testing.T) {
	require.PanicsWithValue(t, `unknown rule type "rules.unknownRule"`, func() {
		NewRuleDetail(unknownRule{})
	})
}

func TestFromOriginContext(t *testing.T) {
	t.Run("should return zero value if RuleDetail is missing in the context", func(t *testing.T) {
		detail := FromOriginContext(context.Background())
		require.Zero(t, detail)

		// The zero value for NoDependentRules must be the most conservative option.
		require.False(t, detail.NoDependentRules)

		// The zero value for NoDependencyRules must be the most conservative option.
		require.False(t, detail.NoDependencyRules)
	})
}

func TestNewRuleDetail(t *testing.T) {
	t.Run("should populate NoDependentRules and NoDependencyRules for a RecordingRule", func(t *testing.T) {
		rule := NewRecordingRule("test", &parser.NumberLiteral{Val: 1}, labels.EmptyLabels())
		detail := NewRuleDetail(rule)
		require.False(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetDependentRules([]Rule{})
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetDependencyRules([]Rule{})
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.True(t, detail.NoDependencyRules)
	})

	t.Run("should populate NoDependentRules and NoDependencyRules for a AlertingRule", func(t *testing.T) {
		rule := NewAlertingRule(
			"test",
			&parser.NumberLiteral{Val: 1},
			time.Minute,
			0,
			labels.FromStrings("test", "test"),
			labels.EmptyLabels(),
			labels.EmptyLabels(),
			"",
			true, promslog.NewNopLogger(),
		)

		detail := NewRuleDetail(rule)
		require.False(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetDependentRules([]Rule{})
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetDependencyRules([]Rule{})
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.True(t, detail.NoDependencyRules)
	})
}
