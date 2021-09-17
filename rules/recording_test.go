// Copyright 2013 The Prometheus Authors
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
	"html/template"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestRuleEval(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}

	engine := promql.NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	now := time.Now()

	suite := []struct {
		name   string
		expr   parser.Expr
		labels labels.Labels
		limit  int
		result promql.Vector
		err    string
	}{
		{
			name:   "nolabels",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.Labels{},
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "nolabels"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
		{
			name:   "labels",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.FromStrings("foo", "bar"),
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "labels", "foo", "bar"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
		{
			name:   "underlimit",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.FromStrings("foo", "bar"),
			limit:  2,
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "underlimit", "foo", "bar"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
		{
			name:   "atlimit",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.FromStrings("foo", "bar"),
			limit:  1,
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "atlimit", "foo", "bar"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
		{
			name:   "overlimit",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.FromStrings("foo", "bar"),
			limit:  -1,
			err:    "exceeded limit -1 with 1 samples",
		},
	}

	for _, test := range suite {
		rule := NewRecordingRule(test.name, test.expr, test.labels)
		result, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil, test.limit)
		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.Equal(t, test.err, err.Error())
		}
		require.Equal(t, test.result, result)
	}
}

func TestRecordingRuleHTMLSnippet(t *testing.T) {
	expr, err := parser.ParseExpr(`foo{html="<b>BOLD<b>"}`)
	require.NoError(t, err)
	rule := NewRecordingRule("testrule", expr, labels.FromStrings("html", "<b>BOLD</b>"))

	const want = template.HTML(`record: <a href="/test/prefix/graph?g0.expr=testrule&g0.tab=1">testrule</a>
expr: <a href="/test/prefix/graph?g0.expr=foo%7Bhtml%3D%22%3Cb%3EBOLD%3Cb%3E%22%7D&g0.tab=1">foo{html=&#34;&lt;b&gt;BOLD&lt;b&gt;&#34;}</a>
labels:
  html: '&lt;b&gt;BOLD&lt;/b&gt;'
`)

	got := rule.HTMLSnippet("/test/prefix")
	require.Equal(t, want, got, "incorrect HTML snippet; want:\n\n%s\n\ngot:\n\n%s", want, got)
}

// TestRuleEvalDuplicate tests for duplicate labels in recorded metrics, see #5529.
func TestRuleEvalDuplicate(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}

	engine := promql.NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	now := time.Now()

	expr, _ := parser.ParseExpr(`vector(0) or label_replace(vector(0),"test","x","","")`)
	rule := NewRecordingRule("foo", expr, labels.FromStrings("test", "test"))
	_, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil, 0)
	require.Error(t, err)
	require.EqualError(t, err, "vector contains metrics with the same labelset after applying rule labels")
}
