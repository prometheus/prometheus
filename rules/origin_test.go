package rules

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

type unknownRule struct{}

func (u unknownRule) Name() string          { return "" }
func (u unknownRule) Labels() labels.Labels { return nil }
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
