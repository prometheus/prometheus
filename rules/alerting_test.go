// Copyright 2016 The Prometheus Authors
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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
)

func TestAlertingRuleHTMLSnippet(t *testing.T) {
	expr, err := promql.ParseExpr(`foo{html="<b>BOLD<b>"}`)
	if err != nil {
		t.Fatal(err)
	}
	rule := NewAlertingRule("testrule", expr, 0, model.LabelSet{"html": "<b>BOLD</b>"}, model.LabelSet{"html": "<b>BOLD</b>"})

	const want = `ALERT <a href="/test/prefix/graph?g0.expr=ALERTS%7Balertname%3D%22testrule%22%7D&g0.tab=0">testrule</a>
  IF <a href="/test/prefix/graph?g0.expr=foo%7Bhtml%3D%22%3Cb%3EBOLD%3Cb%3E%22%7D&g0.tab=0">foo{html=&#34;&lt;b&gt;BOLD&lt;b&gt;&#34;}</a>
  LABELS {html=&#34;&lt;b&gt;BOLD&lt;/b&gt;&#34;}
  ANNOTATIONS {html=&#34;&lt;b&gt;BOLD&lt;/b&gt;&#34;}`

	got := rule.HTMLSnippet("/test/prefix")
	if got != want {
		t.Fatalf("incorrect HTML snippet; want:\n\n|%v|\n\ngot:\n\n|%v|", want, got)
	}
}
