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

// requireAlternatives asserts whether the root AND keeps score alternatives,
// so a case meant for one evaluation path cannot silently test the other.
func requireAlternatives(t testing.TB, filter storage.Filter, want bool) {
	t.Helper()
	_, got := filter.(*compiledFilter).root.(*bestAlternative)
	require.Equal(t, want, got, "root keeps score alternatives")
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
		scores     map[string]float64
	}{
		{name: "AND is the mean of both terms", expression: "foo AND bar OR baz", value: "foo_bar", wantScore: 0.6},
		{name: "OR picks the operand with the higher mean", expression: "foo AND bar OR baz", value: "foo_bar_baz", wantScore: 0.7},
		{name: "OR with one accepted operand", expression: "foo AND bar OR baz", value: "baz", wantScore: 0.7},
		{name: "AND does not use the lowest term", expression: "low AND high", value: "low_high", wantScore: 0.5},
		{name: "OR keeps the left operand when it is higher", expression: "high OR low", value: "high_low", wantScore: 0.8},
		{name: "nested AND includes every term", expression: "foo AND (bar AND high)", value: "foo_bar_high", wantScore: (0.8 + 0.4 + 0.8) / 3},
		{name: "unscored subtree adds no term", expression: "(NOT old AND NOT stale) AND foo", value: "foo", wantScore: 0.8},
		{name: "unscored OR subtree adds no term", expression: "(NOT old OR NOT stale) AND foo", value: "foo_old", wantScore: 0.8},
		{name: "OR takes a positive operand that raises the mean", expression: "bar AND (foo OR NOT old)", value: "bar_foo", wantScore: 0.6},
		// Equal OR means can carry different term counts into a surrounding AND.
		// Both operand orders must retain the best complete-path mean.
		{name: "OR keeps both counts under AND", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 1.75 / 3, scores: map[string]float64{"a": 0.75, "b": 0.75, "c": 0.75, "d": 0.25}},
		{name: "reversed OR keeps both counts under AND", expression: "((b AND c) OR a) AND d", value: "a_b_c_d", wantScore: 1.75 / 3, scores: map[string]float64{"a": 0.75, "b": 0.75, "c": 0.75, "d": 0.25}},
		{name: "De Morgan rewrite keeps both counts under AND", expression: "NOT (NOT (a OR (b AND c)) OR NOT d)", value: "a_b_c_d", wantScore: 1.75 / 3, scores: map[string]float64{"a": 0.75, "b": 0.75, "c": 0.75, "d": 0.25}},
		{name: "perfect OR keeps both counts under AND", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 0.75, scores: map[string]float64{"a": 1, "b": 1, "c": 1, "d": 0.25}},
		{name: "reversed perfect OR keeps both counts under AND", expression: "((b AND c) OR a) AND d", value: "a_b_c_d", wantScore: 0.75, scores: map[string]float64{"a": 1, "b": 1, "c": 1, "d": 0.25}},
		// Preferring the larger count on ties also fails: a strong surrounding
		// term makes the smaller branch preferable instead.
		{name: "strong conjunct prefers fewer terms", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 0.875, scores: map[string]float64{"a": 0.75, "b": 0.75, "c": 0.75, "d": 1}},
		// A locally better mean can produce a worse complete-path mean even
		// without a tie, so tie-breaking alone cannot fix score composition.
		{name: "lower local mean wins with more terms", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 1.7 / 3, scores: map[string]float64{"a": 0.9, "b": 0.8, "c": 0.8, "d": 0.1}},
		{name: "before improving OR terms", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 0.75, scores: map[string]float64{"a": 0.5, "b": 0.49, "c": 0.49, "d": 1}},
		{name: "improving OR terms cannot lower score", expression: "(a OR (b AND c)) AND d", value: "a_b_c_d", wantScore: 0.75, scores: map[string]float64{"a": 0.5, "b": 0.51, "c": 0.51, "d": 1}},
		// An exclusion adds no term, so also accepting a weak positive operand
		// cannot lower the score below that of a value without it.
		{name: "OR takes an exclusion over a weaker positive operand", expression: "bar AND (foo OR NOT old)", value: "bar_foo", wantScore: 0.9, scores: map[string]float64{"bar": 0.9, "foo": 0.1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			scores := test.scores
			if scores == nil {
				scores = map[string]float64{"foo": 0.8, "bar": 0.4, "baz": 0.7, "low": 0.2, "high": 0.8}
			}
			filter := compileContains(t, test.expression, scores)
			accepted, score := filter.Accept(test.value)
			require.True(t, accepted)
			require.InDelta(t, test.wantScore, score, 1e-9)
		})
	}
}

func TestCompileScoresAtTermLimit(t *testing.T) {
	// Ten independent OR choices have 1024 accepting paths but only eleven
	// possible counts. The evaluator must retain those counts within the limit.
	expression := strings.Repeat("(a OR (b AND c)) AND ", 10) + "d AND e"
	for _, test := range []struct {
		name      string
		branch    float64
		conjunct  float64
		wantScore float64
	}{
		{name: "weak conjuncts prefer more terms", branch: 0.75, conjunct: 0.25, wantScore: 15.5 / 22},
		{name: "strong conjuncts prefer fewer terms", branch: 0.75, conjunct: 1, wantScore: 9.5 / 12},
		{name: "perfect branches retain their counts", branch: 1, conjunct: 0.25, wantScore: 20.5 / 22},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter := compileContains(t, expression, map[string]float64{
				"a": test.branch, "b": test.branch, "c": test.branch,
				"d": test.conjunct, "e": test.conjunct,
			})
			accepted, score := filter.Accept("a_b_c_d_e")
			require.True(t, accepted)
			require.InDelta(t, test.wantScore, score, 1e-9)
		})
	}
}

func TestCompileEquivalentExpressionsRankIdentically(t *testing.T) {
	// The other candidate lies between the two scores produced by the old
	// operand-order-dependent evaluation, so Limit=1 exposed different results.
	for _, expression := range []string{
		"((a OR (b AND c)) AND d) OR other",
		"(((b AND c) OR a) AND d) OR other",
	} {
		t.Run(expression, func(t *testing.T) {
			filter := compileContains(t, expression, map[string]float64{"a": 0.75, "b": 0.75, "c": 0.75, "d": 0.25, "other": 0.55})
			results := storage.ApplySearchHints([]string{"a_b_c_d", "other"}, &storage.SearchHints{
				Filter: filter, Limit: 1, OrderBy: storage.OrderByScoreDesc,
			})
			require.Equal(t, []storage.SearchResult{{Value: "a_b_c_d", Score: 1.75 / 3}}, results)
		})
	}
}

func TestCompileAcceptIsSafeForConcurrentUse(t *testing.T) {
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

	// Exercise both evaluation paths; score alternatives must remain
	// request-local.
	for _, test := range []struct {
		expression   string
		alternatives bool
	}{
		{expression: "foo OR bar"},
		{expression: "(foo OR (bar AND bar)) AND NOT old", alternatives: true},
	} {
		t.Run(test.expression, func(t *testing.T) {
			filter := compileContains(t, test.expression, map[string]float64{"foo": 0.9, "bar": 0.3})
			requireAlternatives(t, filter, test.alternatives)
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
		})
	}
}

func TestCompileAndStopsAtFirstRejection(t *testing.T) {
	// Both the scalar and alternative-score evaluators must stop on rejection.
	for _, test := range []struct {
		expression   string
		alternatives bool
	}{
		{expression: "first AND later"},
		{expression: "first AND (later OR (a AND b))", alternatives: true},
	} {
		t.Run(test.expression, func(t *testing.T) {
			calls := map[string]int{}
			expr, err := Parse(test.expression)
			require.NoError(t, err)
			filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
				return countingTermFilter{accepted: term != "first", score: 0.8, calls: calls, term: term}, nil
			})
			require.NoError(t, err)
			requireAlternatives(t, filter, test.alternatives)

			accepted, score := filter.Accept("anything")
			require.False(t, accepted)
			require.Zero(t, score)
			require.Equal(t, map[string]int{"first": 1}, calls)
		})
	}
}

func TestCompileOrStopsOnlyAtPerfectPositiveMatch(t *testing.T) {
	for _, test := range []struct {
		name       string
		expression string
		scores     map[string]float64
		rejected   string
		wantScore  float64
		wantCalls  string
	}{
		{name: "scalar perfect match", expression: "perfect OR later", wantScore: 1, wantCalls: "perfect"},
		// The unused subtree keeps score alternatives. A perfect root match
		// must still skip all of it.
		{name: "perfect root match", expression: "perfect OR (anchor AND (a OR (b AND c)))", wantScore: 1, wantCalls: "perfect"},
		{name: "chained root ORs", expression: "perfect OR later OR (anchor AND (a OR (b AND c)))", wantScore: 1, wantCalls: "perfect"},
		{name: "De Morgan root OR", expression: "NOT (NOT perfect AND NOT (anchor AND (a OR (b AND c))))", wantScore: 1, wantCalls: "perfect"},
		// The perfect alternative need not have the smallest positive count.
		{name: "perfect path among several counts", expression: "(a AND (b OR (c AND d))) OR later", scores: map[string]float64{"b": 0.5}, wantScore: 1, wantCalls: "a b c d"},
		// An enclosing AND needs the larger-count perfect path to maximize its
		// final mean. OR must preserve that context through either child.
		{name: "perfect OR under AND", expression: "anchor AND (perfect OR (b AND c))", scores: map[string]float64{"anchor": 0.25}, wantScore: 0.75, wantCalls: "anchor perfect b c"},
		{name: "left nested OR under AND", expression: "anchor AND ((perfect OR (b AND c)) OR later)", scores: map[string]float64{"anchor": 0.25}, wantScore: 0.75, wantCalls: "anchor perfect b c later"},
		{name: "right nested OR under AND", expression: "anchor AND (perfect OR (later OR (b AND c)))", scores: map[string]float64{"anchor": 0.25}, wantScore: 0.75, wantCalls: "anchor perfect later b c"},
		{name: "De Morgan enclosing AND", expression: "NOT (NOT anchor OR NOT (perfect OR (b AND c)))", scores: map[string]float64{"anchor": 0.25}, wantScore: 0.75, wantCalls: "anchor perfect b c"},
		// Rejection and non-perfect scores leave room for the other operand.
		{name: "rejected left operand", expression: "perfect OR (anchor AND (a OR (b AND c)))", rejected: "perfect", wantScore: 1, wantCalls: "perfect anchor a b c"},
		{name: "zero left score", expression: "perfect OR (anchor AND (a OR (b AND c)))", scores: map[string]float64{"perfect": 0}, wantScore: 1, wantCalls: "perfect anchor a b c"},
		{name: "non-perfect left score", expression: "perfect OR (anchor AND (a OR (b AND c)))", scores: map[string]float64{"perfect": 0.5}, wantScore: 1, wantCalls: "perfect anchor a b c"},
		// An accepted exclusion adds no term, but must not hide a later
		// positive operand that raises the mean.
		{name: "accepted exclusion under AND", expression: "anchor AND (NOT old OR later)", scores: map[string]float64{"anchor": 0.5}, rejected: "old", wantScore: 0.75, wantCalls: "anchor old later"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := map[string]int{}
			expr, err := Parse(test.expression)
			require.NoError(t, err)
			filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
				score := 1.0
				if override, ok := test.scores[term]; ok {
					score = override
				}
				return countingTermFilter{accepted: term != test.rejected, score: score, calls: calls, term: term}, nil
			})
			require.NoError(t, err)

			accepted, score := filter.Accept("anything")
			require.True(t, accepted)
			require.Equal(t, test.wantScore, score)
			wantCalls := map[string]int{}
			for term := range strings.FieldsSeq(test.wantCalls) {
				wantCalls[term]++
			}
			require.Equal(t, wantCalls, calls)
		})
	}
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
	for _, test := range []struct {
		name       string
		expression string
		values     []string
		scores     map[string]float64
	}{
		{name: "simple", expression: "foo AND bar OR baz", values: []string{"foo_bar", "foo_bar_baz", "baz_only", "no_match"}},
		{name: "nested_or", expression: "(foo OR (bar AND baz)) AND end", values: []string{"foo_bar_end", "foo_bar_baz_end", "baz_only", "no_match"}},
		{name: "term_limit", expression: strings.Repeat("(foo OR (bar AND baz)) AND ", 10) + "end AND last", values: []string{"foo_bar_end_last", "foo_bar_baz_end_last", "baz_only", "no_match"}},
		// Every leaf matches these candidates, so rejection cannot hide work in
		// the right branch that a perfect left match should skip.
		{name: "perfect_root", expression: "perfect OR (anchor AND (a OR (b AND c)))", values: []string{"perfect_anchor_a_b_c"}, scores: map[string]float64{"perfect": 1, "anchor": 1, "a": 1, "b": 1, "c": 1}},
	} {
		b.Run(test.name, func(b *testing.B) {
			scores := test.scores
			if scores == nil {
				scores = map[string]float64{"foo": 0.7, "bar": 0.7, "baz": 0.7, "end": 0.25, "last": 0.25}
			}
			filter := compileContains(b, test.expression, scores)
			i := 0
			for b.Loop() {
				filter.Accept(test.values[i%len(test.values)])
				i++
			}
		})
	}
}

// scoredAcceptingPaths enumerates leaf scores instead of pruning by term
// count. It is an independent score oracle for expressions with few terms.
func scoredAcceptingPaths(expr Expr, value string, negated bool, scores map[string]float64) [][]float64 {
	var left, right Expr
	var conjunction bool
	switch expr := expr.(type) {
	case Term:
		accepted := strings.Contains(value, expr.Value)
		if negated {
			if !accepted {
				return [][]float64{nil}
			}
		} else if accepted {
			return [][]float64{{scores[expr.Value]}}
		}
		return nil
	case Not:
		return scoredAcceptingPaths(expr.Expr, value, !negated, scores)
	case And:
		left, right, conjunction = expr.Left, expr.Right, !negated
	case Or:
		left, right, conjunction = expr.Left, expr.Right, negated
	default:
		panic("unknown node")
	}

	leftPaths := scoredAcceptingPaths(left, value, negated, scores)
	rightPaths := scoredAcceptingPaths(right, value, negated, scores)
	if !conjunction {
		return append(leftPaths, rightPaths...)
	}

	var paths [][]float64
	for _, l := range leftPaths {
		for _, r := range rightPaths {
			// Copy the left path so extending one alternative cannot overwrite another.
			path := append([]float64(nil), l...)
			paths = append(paths, append(path, r...))
		}
	}
	return paths
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
		"(a OR (b AND c)) AND d",
		"((b AND c) OR a) AND d",
		"NOT (NOT (a OR (b AND c)) OR NOT d)",
	} {
		f.Add(input)
	}

	values := []string{"", "foo_new_metric", "foo_old", "old metric", "AND", "f", "a_b_c_d", "foo_bar_baz"}
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

		termCount := 0
		scores := map[string]float64{}
		filter, err := Compile(expr, func(term string, _ bool) (storage.Filter, error) {
			termCount++
			// Vary scores deterministically, including zero and perfect matches.
			scores[term] = float64((len(term)+int(term[0]))%9) / 8
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

		// Exhaustive paths can grow exponentially; the production evaluator
		// remains bounded by term count. Keep this oracle's inputs small.
		if termCount > 8 {
			return
		}
		filter, err = Compile(expr, containsFactory(scores))
		require.NoError(t, err)
		for _, value := range values {
			paths := scoredAcceptingPaths(expr, value, false, scores)
			var wantScore float64
			for _, path := range paths {
				require.NotEmpty(t, path, "validated accepting paths require a positive term")
				var sum float64
				for _, score := range path {
					sum += score
				}
				wantScore = max(wantScore, sum/float64(len(path)))
			}
			accepted, score := filter.Accept(value)
			require.Equal(t, len(paths) > 0, accepted)
			require.InDelta(t, wantScore, score, 1e-9)
		}
	})
}
