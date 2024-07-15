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

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

type unknownRule struct{}

func (u unknownRule) Name() string          { return "" }
func (u unknownRule) Labels() labels.Labels { return labels.EmptyLabels() }
func (u unknownRule) Eval(context.Context, time.Duration, time.Time, QueryFunc, *url.URL, int) (promql.Vector, error) {
	return nil, nil
}
func (u unknownRule) String() string                       { return "" }
func (u unknownRule) Query() parser.Expr                   { return nil }
func (u unknownRule) SetLastError(error)                   {}
func (u unknownRule) LastError() error                     { return nil }
func (u unknownRule) SetHealth(RuleHealth)                 {}
func (u unknownRule) Health() RuleHealth                   { return "" }
func (u unknownRule) SetEvaluationDuration(time.Duration)  {}
func (u unknownRule) GetEvaluationDuration() time.Duration { return 0 }
func (u unknownRule) SetEvaluationTimestamp(time.Time)     {}
func (u unknownRule) GetEvaluationTimestamp() time.Time    { return time.Time{} }
func (u unknownRule) SetNoDependentRules(bool)             {}
func (u unknownRule) NoDependentRules() bool               { return false }
func (u unknownRule) SetNoDependencyRules(bool)            {}
func (u unknownRule) NoDependencyRules() bool              { return false }

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

		rule.SetNoDependentRules(true)
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetNoDependencyRules(true)
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
			true, log.NewNopLogger(),
		)

		detail := NewRuleDetail(rule)
		require.False(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetNoDependentRules(true)
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.False(t, detail.NoDependencyRules)

		rule.SetNoDependencyRules(true)
		detail = NewRuleDetail(rule)
		require.True(t, detail.NoDependentRules)
		require.True(t, detail.NoDependencyRules)
	})
}
