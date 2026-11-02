// Copyright The Prometheus Authors
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

package searchexpr

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
)

type containsFilter struct {
	term  string
	score float64
}

func (f containsFilter) Accept(value string) (bool, float64) {
	if !strings.Contains(value, f.term) {
		return false, 0
	}
	return true, f.score
}

type countingTermFilter struct {
	accepted bool
	score    float64
	calls    map[string]int
	term     string
}

func (f countingTermFilter) Accept(string) (bool, float64) {
	f.calls[f.term]++
	return f.accepted, f.score
}

type unknownExpr struct{}

func (unknownExpr) expr() {}

func containsFactory(scores map[string]float64) TermFilterFactory {
	return func(term string, _ bool) (storage.Filter, error) {
		return containsFilter{term: term, score: scores[term]}, nil
	}
}

func compileContains(t testing.TB, input string, scores map[string]float64) storage.Filter {
	t.Helper()
	expr, err := Parse(input)
	require.NoError(t, err)
	filter, err := Compile(expr, containsFactory(scores))
	require.NoError(t, err)
	return filter
}

func TestCompileNotExpressions(t *testing.T) {
	for _, test := range []struct {
		name       string
		expression string
		value      string
		wantAccept bool
		wantScore  float64
	}{
		{name: "positive AND negative keeps positive score", expression: "foo AND NOT old", value: "foo_new", wantAccept: true, wantScore: 0.8},
		{name: "positive AND negative rejects excluded value", expression: "foo AND NOT old", value: "foo_old"},
		{name: "double negation keeps positive score", expression: "NOT NOT foo", value: "foo_new", wantAccept: true, wantScore: 0.8},
		{name: "double negation rejects a non-match", expression: "NOT NOT foo", value: "brand_new"},
		{name: "nested negation keeps positive branch score", expression: "NOT (NOT foo OR old)", value: "foo_new", wantAccept: true, wantScore: 0.8},
		{name: "nested negation rejects excluded value", expression: "NOT (NOT foo OR old)", value: "foo_old"},
		{name: "negated AND accepts value missing one term", expression: "foo AND NOT (old AND stale)", value: "foo_old", wantAccept: true, wantScore: 0.8},
		{name: "negated AND rejects value with both terms", expression: "foo AND NOT (old AND stale)", value: "foo_old_stale"},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter := compileContains(t, test.expression, map[string]float64{"foo": 0.8, "old": 0.6, "stale": 0.5})
			accepted, score := filter.Accept(test.value)
			require.Equal(t, test.wantAccept, accepted)
			require.InDelta(t, test.wantScore, score, 1e-9)
		})
	}
}

func TestCompileScoresMeanOfPositiveTerms(t *testing.T) {
	for _, test := range []struct {
		name       string
		expression string
		value      string
		wantScore  float64
	}{
		{name: "AND is the mean of both terms", expression: "foo AND bar OR baz", value: "foo_bar", wantScore: 0.6},
		{name: "OR picks the operand with the higher mean", expression: "foo AND bar OR baz", value: "foo_bar_baz", wantScore: 0.7},
		{name: "OR with one accepted operand", expression: "foo AND bar OR baz", value: "baz", wantScore: 0.7},
		{name: "AND does not use the lowest term", expression: "low AND high", value: "low_high", wantScore: 0.5},
		{name: "OR keeps the left operand when it is higher", expression: "high OR low", value: "high_low", wantScore: 0.8},
		{name: "nested AND includes every term", expression: "foo AND (bar AND high)", value: "foo_bar_high", wantScore: (0.8 + 0.4 + 0.8) / 3},
		{name: "unscored subtree adds no term", expression: "(NOT old AND NOT stale) AND foo", value: "foo", wantScore: 0.8},
		{name: "unscored OR subtree adds no term", expression: "(NOT old OR NOT stale) AND foo", value: "foo_old", wantScore: 0.8},
		{name: "OR keeps the scored operand over an unscored one", expression: "bar AND (foo OR NOT old)", value: "bar_foo", wantScore: 0.6},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter := compileContains(t, test.expression, map[string]float64{
				"foo":  0.8,
				"bar":  0.4,
				"baz":  0.7,
				"low":  0.2,
				"high": 0.8,
			})
			accepted, score := filter.Accept(test.value)
			require.True(t, accepted)
			require.InDelta(t, test.wantScore, score, 1e-9)
		})
	}
}

func TestCompileAcceptIsSafeForConcurrentUse(t *testing.T) {
	filter := compileContains(t, "foo OR bar", map[string]float64{"foo": 0.9, "bar": 0.3})

	cases := []struct {
		value      string
		wantAccept bool
		wantScore  float64
	}{
		{value: "foo_only", wantAccept: true, wantScore: 0.9},
		{value: "bar_only", wantAccept: true, wantScore: 0.3},
		{value: "foo_bar", wantAccept: true, wantScore: 0.9},
		{value: "neither"},
	}

	var wg sync.WaitGroup
	for range 50 {
		for _, c := range cases {
			wg.Go(func() {
				accepted, score := filter.Accept(c.value)
				require.Equal(t, c.wantAccept, accepted, c.value)
				require.InDelta(t, c.wantScore, score, 1e-9, c.value)
			})
		}
	}
	wg.Wait()
}

func TestCompileAndStopsAtFirstRejection(t *testing.T) {
	calls := map[string]int{}
	expr, err := Parse("first AND later")
	require.NoError(t, err)
	filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
		return countingTermFilter{accepted: term != "first", score: 0.8, calls: calls, term: term}, nil
	})
	require.NoError(t, err)

	accepted, score := filter.Accept("anything")
	require.False(t, accepted)
	require.Zero(t, score)
	require.Equal(t, 1, calls["first"])
	require.Zero(t, calls["later"])
}

func TestCompileOrStopsOnlyAtPerfectPositiveMatch(t *testing.T) {
	t.Run("positive perfect match", func(t *testing.T) {
		calls := map[string]int{}
		expr, err := Parse("perfect OR later")
		require.NoError(t, err)
		filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
			score := 0.5
			if term == "perfect" {
				score = 1
			}
			return countingTermFilter{accepted: true, score: score, calls: calls, term: term}, nil
		})
		require.NoError(t, err)

		accepted, score := filter.Accept("anything")
		require.True(t, accepted)
		require.Equal(t, 1.0, score)
		require.Equal(t, 1, calls["perfect"])
		require.Zero(t, calls["later"])
	})

	t.Run("unscored negative match", func(t *testing.T) {
		calls := map[string]int{}
		expr, err := Parse("anchor AND (NOT old OR later)")
		require.NoError(t, err)
		filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
			switch term {
			case "anchor":
				return containsFilter{term: "anything", score: 0.9}, nil
			case "old":
				return containsFilter{term: "never", score: 1}, nil
			default:
				return countingTermFilter{accepted: true, score: 0.6, calls: calls, term: term}, nil
			}
		})
		require.NoError(t, err)

		accepted, score := filter.Accept("anything")
		require.True(t, accepted)
		require.InDelta(t, 0.75, score, 1e-9, "mean of anchor and later; the NOT operand adds no term")
		require.Equal(t, 1, calls["later"], "an unscored NOT match must not hide a later positive score")
	})
}

func TestCompileReportsEffectiveTermPolarity(t *testing.T) {
	expr, err := Parse("anchor AND NOT old AND NOT NOT restored AND NOT (gone OR NOT back)")
	require.NoError(t, err)

	polarities := map[string]bool{}
	_, err = Compile(expr, func(term string, negated bool) (storage.Filter, error) {
		polarities[term] = negated
		return containsFilter{term: term, score: 1}, nil
	})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{
		"anchor":   false,
		"old":      true,
		"restored": false,
		"gone":     true,
		"back":     false,
	}, polarities)
}

func TestCompileRejectsInvalidInputs(t *testing.T) {
	factoryErr := errors.New("factory failed")
	validFactory := containsFactory(nil)

	for _, test := range []struct {
		name    string
		expr    Expr
		factory TermFilterFactory
		want    string
	}{
		{name: "nil expression", factory: validFactory, want: "search expression: cannot compile a nil expression"},
		{name: "nil factory", expr: Term{Value: "foo"}, want: "search expression: term filter factory is nil"},
		{name: "empty term", expr: Term{}, factory: validFactory, want: "search expression: cannot compile an empty term"},
		{name: "nil NOT operand", expr: And{Left: Term{Value: "foo"}, Right: Not{}}, factory: validFactory, want: "search expression: cannot compile NOT with a nil operand"},
		{name: "nil AND operand", expr: And{Left: Term{Value: "foo"}}, factory: validFactory, want: "search expression: cannot compile a binary expression with a nil operand"},
		{name: "nil OR operand", expr: Or{Right: Term{Value: "foo"}}, factory: validFactory, want: "search expression: cannot compile a binary expression with a nil operand"},
		{name: "unknown node", expr: unknownExpr{}, factory: validFactory, want: "search expression: cannot compile node searchexpr.unknownExpr"},
		{name: "pointer node", expr: &Term{Value: "foo"}, factory: validFactory, want: "search expression: cannot compile node *searchexpr.Term"},
		{
			name: "factory error",
			expr: And{Left: Term{Value: "foo"}, Right: Term{Value: "bar"}},
			factory: func(term string, _ bool) (storage.Filter, error) {
				if term == "bar" {
					return nil, factoryErr
				}
				return containsFilter{term: term}, nil
			},
			want: `search expression: compile term "bar": factory failed`,
		},
		{
			name: "factory error in left OR operand",
			expr: Or{Left: Term{Value: "bar"}, Right: Term{Value: "foo"}},
			factory: func(term string, _ bool) (storage.Filter, error) {
				if term == "bar" {
					return nil, factoryErr
				}
				return containsFilter{term: term}, nil
			},
			want: `search expression: compile term "bar": factory failed`,
		},
		{
			name: "nil term filter",
			expr: Term{Value: "foo"},
			factory: func(string, bool) (storage.Filter, error) {
				return nil, nil
			},
			want: `search expression: term filter factory returned nil for "foo"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter, err := Compile(test.expr, test.factory)
			require.EqualError(t, err, test.want)
			require.Nil(t, filter)
		})
	}
}

func TestCompileWrapsFactoryError(t *testing.T) {
	factoryErr := errors.New("factory failed")
	_, err := Compile(Term{Value: "foo"}, func(string, bool) (storage.Filter, error) {
		return nil, factoryErr
	})
	require.ErrorIs(t, err, factoryErr)
}

func TestValidateRejectsExpressionsWithoutPositiveTerm(t *testing.T) {
	for _, expr := range []Expr{
		Not{Expr: Term{Value: "old"}},
		Or{Left: Term{Value: "foo"}, Right: Not{Expr: Term{Value: "old"}}},
		Not{Expr: Or{Left: Term{Value: "foo"}, Right: Term{Value: "old"}}},
		Not{Expr: And{Left: Term{Value: "foo"}, Right: Term{Value: "old"}}},
		Or{
			Left:  And{Left: Term{Value: "foo"}, Right: Not{Expr: Term{Value: "old"}}},
			Right: Not{Expr: Term{Value: "stale"}},
		},
	} {
		require.EqualError(t, Validate(expr), "search expression: every accepting path must require a positive term")
		filter, err := Compile(expr, containsFactory(nil))
		require.EqualError(t, err, "search expression: every accepting path must require a positive term")
		require.Nil(t, filter)
	}
}

func TestValidateAcceptsExpressionsWithPositiveTerm(t *testing.T) {
	for _, input := range []string{
		"foo",
		"NOT NOT foo",
		"foo AND NOT old",
		"NOT (NOT foo OR old)",
		"NOT (old AND NOT foo) AND bar",
		"(NOT old OR NOT stale) AND foo",
	} {
		expr, err := Parse(input)
		require.NoError(t, err)
		require.NoError(t, Validate(expr), input)
	}
}

func TestValidateRejectsASTOutsideResourceLimits(t *testing.T) {
	tooManyTerms := Expr(Term{Value: "term"})
	for range maxExpressionTerms {
		tooManyTerms = And{Left: tooManyTerms, Right: Term{Value: "term"}}
	}
	require.EqualError(t, Validate(tooManyTerms), "search expression: term count exceeds maximum of 32")

	tooDeep := Expr(Term{Value: "foo"})
	for range maxCompiledExpressionDepth {
		tooDeep = Not{Expr: tooDeep}
	}
	require.EqualError(t, Validate(tooDeep), "search expression: AST depth exceeds maximum of 48")
}

func TestCompileAcceptsDeepestParsedExpression(t *testing.T) {
	terms := make([]string, maxExpressionTerms)
	for i := range terms {
		terms[i] = "term"
	}
	input := strings.Repeat("NOT ", maxExpressionNestingDepth) + strings.Join(terms, " AND ")
	expr, err := Parse(input)
	require.NoError(t, err)
	require.Equal(t, maxCompiledExpressionDepth, exprDepth(expr))

	filter, err := Compile(expr, containsFactory(map[string]float64{"term": 1}))
	require.NoError(t, err)
	accepted, _ := filter.Accept("other")
	require.False(t, accepted)
}

func exprDepth(expr Expr) int {
	switch expr := expr.(type) {
	case And:
		return 1 + max(exprDepth(expr.Left), exprDepth(expr.Right))
	case Or:
		return 1 + max(exprDepth(expr.Left), exprDepth(expr.Right))
	case Not:
		return 1 + exprDepth(expr.Expr)
	default:
		return 1
	}
}

// evaluateDirectly evaluates expr without the De Morgan rewrite.
func evaluateDirectly(expr Expr, value string) bool {
	switch expr := expr.(type) {
	case Term:
		return strings.Contains(value, expr.Value)
	case And:
		return evaluateDirectly(expr.Left, value) && evaluateDirectly(expr.Right, value)
	case Or:
		return evaluateDirectly(expr.Left, value) || evaluateDirectly(expr.Right, value)
	case Not:
		return !evaluateDirectly(expr.Expr, value)
	default:
		panic("unknown node")
	}
}

func BenchmarkCompiledFilterAccept(b *testing.B) {
	filter := compileContains(b, "foo AND bar OR baz", map[string]float64{"foo": 0.7, "bar": 0.7, "baz": 0.7})
	values := []string{"foo_bar", "foo_bar_baz", "baz_only", "no_match"}

	i := 0
	for b.Loop() {
		filter.Accept(values[i%len(values)])
		i++
	}
}

func FuzzCompile(f *testing.F) {
	for _, input := range []string{
		"foo",
		"NOT foo",
		"foo AND NOT old",
		"NOT NOT foo",
		"NOT (foo OR old)",
		"NOT (NOT foo OR old)",
		"(NOT o OR NOT n) AND f",
		`"AND" OR NOT "old metric"`,
	} {
		f.Add(input)
	}

	values := []string{"", "foo_new_metric", "foo_old", "old metric", "AND", "f"}
	f.Fuzz(func(t *testing.T, input string) {
		expr, err := Parse(input)
		if err != nil {
			return
		}
		if Validate(expr) != nil {
			_, err := Compile(expr, containsFactory(nil))
			require.Error(t, err)
			return
		}

		filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
			return containsFilter{term: term, score: 0.5}, nil
		})
		require.NoError(t, err)
		for _, value := range values {
			accepted, score := filter.Accept(value)
			require.Equal(t, evaluateDirectly(expr, value), accepted, value)
			if accepted {
				require.InDelta(t, 0.5, score, 1e-9, "every positive term scores 0.5, so every mean is 0.5")
			} else {
				require.Zero(t, score)
			}
		}
	})
}
