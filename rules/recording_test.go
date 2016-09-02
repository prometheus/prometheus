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
	"reflect"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
)

func TestRuleEval(t *testing.T) {
	storage, closer := local.NewTestStorage(t, 2)
	defer closer.Close()
	engine := promql.NewEngine(storage, nil)
	now := model.Now()

	suite := []struct {
		name   string
		expr   promql.Expr
		labels model.LabelSet
		result model.Vector
	}{
		{
			name:   "nolabels",
			expr:   &promql.NumberLiteral{Val: 1},
			labels: model.LabelSet{},
			result: model.Vector{&model.Sample{
				Value:     1,
				Timestamp: now,
				Metric:    model.Metric{"__name__": "nolabels"},
			}},
		},
		{
			name:   "labels",
			expr:   &promql.NumberLiteral{Val: 1},
			labels: model.LabelSet{"foo": "bar"},
			result: model.Vector{&model.Sample{
				Value:     1,
				Timestamp: now,
				Metric:    model.Metric{"__name__": "labels", "foo": "bar"},
			}},
		},
	}

	for _, test := range suite {
		rule := NewRecordingRule(test.name, test.expr, test.labels)
		result, err := rule.eval(now, engine, "")
		if err != nil {
			t.Fatalf("Error evaluating %s", test.name)
		}
		if !reflect.DeepEqual(result, test.result) {
			t.Fatalf("Error: expected %q, got %q", test.result, result)
		}
	}
}

func TestRecordingRuleHTMLSnippet(t *testing.T) {
	expr, err := promql.ParseExpr(`foo{html="<b>BOLD<b>"}`)
	if err != nil {
		t.Fatal(err)
	}
	rule := NewRecordingRule("testrule", expr, model.LabelSet{"html": "<b>BOLD</b>"})

	// This is valid once the /graph changes have been reintroduced:
	// const want = `<a href="/test/prefix/graph?g0.expr=testrule&g0.tab=0">testrule</a>{html=&#34;&lt;b&gt;BOLD&lt;/b&gt;&#34;} = <a href="/test/prefix/graph?g0.expr=foo%7Bhtml%3D%22%3Cb%3EBOLD%3Cb%3E%22%7D&g0.tab=0">foo{html=&#34;&lt;b&gt;BOLD&lt;b&gt;&#34;}</a>`
	// This is what we need for now:
	const want = `<a href="/test/prefix/graph#%5B%7B%22expr%22%3A%22testrule%22%2C%22tab%22%3A0%7D%5D">testrule</a>{html=&#34;&lt;b&gt;BOLD&lt;/b&gt;&#34;} = <a href="/test/prefix/graph#%5B%7B%22expr%22%3A%22foo%7Bhtml%3D%5C%22%3Cb%3EBOLD%3Cb%3E%5C%22%7D%22%2C%22tab%22%3A0%7D%5D">foo{html=&#34;&lt;b&gt;BOLD&lt;b&gt;&#34;}</a>`

	got := rule.HTMLSnippet("/test/prefix")
	if got != want {
		t.Fatalf("incorrect HTML snippet; want:\n\n%s\n\ngot:\n\n%s", want, got)
	}
}
