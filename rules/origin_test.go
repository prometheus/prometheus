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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

type unknownRule struct{}

func (u unknownRule) Name() string          { return "" }
func (u unknownRule) Labels() labels.Labels { return labels.EmptyLabels() }
func (u unknownRule) Eval(ctx context.Context, time time.Time, queryFunc QueryFunc, url *url.URL, i int) (promql.Vector, error) {
	return nil, nil
}
func (u unknownRule) String() string                               { return "" }
func (u unknownRule) Query() parser.Expr                           { return nil }
func (u unknownRule) SetLastError(err error)                       {}
func (u unknownRule) LastError() error                             { return nil }
func (u unknownRule) SetHealth(health RuleHealth)                  {}
func (u unknownRule) Health() RuleHealth                           { return "" }
func (u unknownRule) SetEvaluationDuration(duration time.Duration) {}
func (u unknownRule) GetEvaluationDuration() time.Duration         { return 0 }
func (u unknownRule) SetEvaluationTimestamp(time time.Time)        {}
func (u unknownRule) GetEvaluationTimestamp() time.Time            { return time.Time{} }

func TestNewRuleDetailPanics(t *testing.T) {
	require.PanicsWithValue(t, `unknown rule type "rules.unknownRule"`, func() {
		NewRuleDetail(unknownRule{})
	})
}
