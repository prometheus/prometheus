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

package parser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestExprString(t *testing.T) {
	ExperimentalDurationExpr = true
	EnableBinopFillModifiers = true
	t.Cleanup(func() {
		ExperimentalDurationExpr = false
		EnableBinopFillModifiers = false
	})
	// A list of valid expressions that are expected to be
	// returned as out when calling String(). If out is empty the output
	// is expected to equal the input.
	inputs := []struct {
		in, out string
	}{
		{
			in:  `sum by() (task:errors:rate10s{job="s"})`,
			out: `sum(task:errors:rate10s{job="s"})`,
		},
		{
			in:  `sum by(code) (task:errors:rate10s{job="s"})`,
			out: `sum by (code) (task:errors:rate10s{job="s"})`,
		},
		{
			in:  `sum without() (task:errors:rate10s{job="s"})`,
			out: `sum without () (task:errors:rate10s{job="s"})`,
		},
		{
			in:  `sum without(instance) (task:errors:rate10s{job="s"})`,
			out: `sum without (instance) (task:errors:rate10s{job="s"})`,
		},
		{
			in:  `sum by("foo.bar") (task:errors:rate10s{job="s"})`,
			out: `sum by ("foo.bar") (task:errors:rate10s{job="s"})`,
		},
		{
			in:  `sum without("foo.bar") (task:errors:rate10s{job="s"})`,
			out: `sum without ("foo.bar") (task:errors:rate10s{job="s"})`,
		},
		{
			in: `topk(5, task:errors:rate10s{job="s"})`,
		},
		{
			in: `count_values("value", task:errors:rate10s{job="s"})`,
		},
		{
			in:  `a - on() c`,
			out: `a - on () c`,
		},
		{
			in:  `a - on(b) c`,
			out: `a - on (b) c`,
		},
		{
			in:  `a - on(b) group_left(x) c`,
			out: `a - on (b) group_left (x) c`,
		},
		{
			in:  `a - on(b) group_left(x, y) c`,
			out: `a - on (b) group_left (x, y) c`,
		},
		{
			in:  `a - on(b) group_left c`,
			out: `a - on (b) group_left () c`,
		},
		{
			in:  `a - on(b) group_left() (c)`,
			out: `a - on (b) group_left () (c)`,
		},
		{
			in:  `a - ignoring(b) c`,
			out: `a - ignoring (b) c`,
		},
		{
			in:  `a - ignoring() c`,
			out: `a - c`,
		},
		{
			// This is a bit of an odd case, but valid. If the user specifies ignoring() with
			// no labels, it means that both label sets have to be exactly the same on both
			// sides (except for the metric name). This is the same behavior as specifying
			// no matching modifier at all, but if the user wants to include the metric name
			// from either side in the output via group_x(__name__), they have to specify
			// ignoring() explicitly to be able to do so, since the grammar does not allow
			// grouping modifiers without either ignoring(...) or on(...). So we need to
			// preserve the empty ignoring() clause in this case.
			//
			//   a - group_left(__name__) c             <--- Parse error
			//   a - ignoring() group_left(__name__) c  <--- Valid
			in:  `a - ignoring() group_left(__metric__) c`,
			out: `a - ignoring () group_left (__metric__) c`,
		},
		{
			in:  `a - ignoring() group_left c`,
			out: `a - ignoring () group_left () c`,
		},
		{
			in:  `a + fill(-23) b`,
			out: `a + fill (-23) b`,
		},
		{
			in:  `a + fill_left(-23) b`,
			out: `a + fill_left (-23) b`,
		},
		{
			in:  `a + fill_right(42) b`,
			out: `a + fill_right (42) b`,
		},
		{
			in:  `a + fill_left(-23) fill_right(42) b`,
			out: `a + fill_left (-23) fill_right (42) b`,
		},
		{
			in:  `a + on(b) group_left fill(-23) c`,
			out: `a + on (b) group_left () fill (-23) c`,
		},
		{
			in: `up > bool 0`,
		},
		{
			in: `a offset 1m`,
		},
		{
			in: `a offset -7m`,
		},
		{
			in: `a{c="d"}[5m] offset 1m`,
		},
		{
			in: `a[5m] offset 1m`,
		},
		{
			in: `a[12m] offset -3m`,
		},
		{
			in: `a[1h:5m] offset 1m`,
		},
		{
			in: `a anchored`,
		},
		{
			in: `a[5m] anchored`,
		},
		{
			in: `a{b="c"}[5m] anchored`,
		},
		{
			in: `a{b="c"}[5m] anchored offset 1m`,
		},
		{
			in: `a{b="c"}[5m] anchored @ start() offset 1m`,
		},
		{
			in: `a smoothed`,
		},
		{
			in: `a[5m] smoothed`,
		},
		{
			in: `a{b="c"}[5m] smoothed`,
		},
		{
			in: `a{b="c"}[5m] smoothed offset 1m`,
		},
		{
			in: `a{b="c"}[5m] smoothed @ start() offset 1m`,
		},
		{
			in: `{__name__="a"}`,
		},
		{
			in: `a{b!="c"}[1m]`,
		},
		{
			in: `a{b=~"c"}[1m]`,
		},
		{
			in: `a{b!~"c"}[1m]`,
		},
		{
			in:  `a @ 10`,
			out: `a @ 10.000`,
		},
		{
			in:  `a[1m] @ 10`,
			out: `a[1m] @ 10.000`,
		},
		{
			in: `a @ start()`,
		},
		{
			in: `a @ end()`,
		},
		{
			in: `a[1m] @ start()`,
		},
		{
			in: `a[1m] @ end()`,
		},
		{
			in: `{__name__="",a="x"}`,
		},
		{
			in: `{"a.b"="c"}`,
		},
		{
			in: `{"0"="1"}`,
		},
		{
			in:  `{"_0"="1"}`,
			out: `{_0="1"}`,
		},
		{
			in: `{""="0"}`,
		},
		{
			in:  "{``=\"0\"}",
			out: `{""="0"}`,
		},
		{
			in:  "1048576",
			out: "1048576",
		},
		{
			in: "foo[step()]",
		},
		{
			in: "foo[-step()]",
		},
		{
			in: "foo[(step())]",
		},
		{
			in: "foo[-(step())]",
		},
		{
			in: "foo offset step()",
		},
		{
			in: "foo offset -step()",
		},
		{
			in: "foo offset (step())",
		},
		{
			in: "foo offset -(step())",
		},
		{
			in:  "foo offset +(5*2)",
			out: "foo offset (5 * 2)",
		},
		{
			in:  "foo offset +min(10s, 20s)",
			out: "foo offset min(10s, 20s)",
		},
		{
			in: "foo offset -min(10s, 20s)",
		},
		{
			in:  "foo offset -min(10s, +max(step() ^ 2, 2))",
			out: "foo offset -min(10s, max(step() ^ 2, 2))",
		},
		{
			in:  "foo[200-min(-step()^+step(),1)]",
			out: "foo[200 - min(-step() ^ step(), 1)]",
		},
		{
			in: "foo[200 - min(step() + 10s, -max(step() ^ 2, 3))]",
		},
		{
			in: "foo[range()]",
		},
		{
			in: "foo[-range()]",
		},
		{
			in: "foo offset range()",
		},
		{
			in: "foo offset -range()",
		},
		{
			in: "foo[max(range(), 5s)]",
		},
		{
			in: `predict_linear(foo[1h], 3000)`,
		},
		{
			in:  `sum by("üüü") (foo)`,
			out: `sum by ("üüü") (foo)`,
		},
		{
			in:  `sum without("äää") (foo)`,
			out: `sum without ("äää") (foo)`,
		},
		{
			in:  `count by("ööö", job) (foo)`,
			out: `count by ("ööö", job) (foo)`,
		},
	}

	EnableExtendedRangeSelectors = true
	t.Cleanup(func() {
		EnableExtendedRangeSelectors = false
	})

	for _, test := range inputs {
		t.Run(test.in, func(t *testing.T) {
			expr, err := ParseExpr(test.in)
			require.NoError(t, err)

			exp := test.in
			if test.out != "" {
				exp = test.out
			}

			require.Equal(t, exp, expr.String())
		})
	}
}

func BenchmarkExprString(b *testing.B) {
	inputs := []string{
		`sum by(code) (task:errors:rate10s{job="s"})`,
		`max( 100 * (1 - avg by(instance) (irate(node_cpu_seconds_total{instance=~".*cust01.prd.*",mode="idle"}[86400s]))))`,
		`http_requests_total{job="api-server", group="canary"} + rate(http_requests_total{job="api-server"}[10m]) * 5 * 60`,
		`sum by (pod) ((kube_pod_container_status_restarts_total{namespace="mynamespace",cluster="mycluster"} - kube_pod_container_status_restarts_total{namespace="mynamespace}",cluster="mycluster}"} offset 10m) >= 1 and ignoring (reason) min_over_time(kube_pod_container_status_last_terminated_reason{namespace="mynamespace",cluster="mycluster",reason="OOMKilled"}[10m]) == 1)`,
		`sum by (pod) ((kube_pod_container_status_restarts_total{cluster="mycluster",namespace="mynamespace"} - kube_pod_container_status_restarts_total{cluster="mycluster",namespace="mynamespace}"} offset 10m) >= 1 and ignoring (reason) min_over_time(kube_pod_container_status_last_terminated_reason{cluster="mycluster",namespace="mynamespace",reason="OOMKilled"}[10m]) == 1)`, // Sort matchers.
		`label_replace(testmetric, "dst", "destination-value-$1", "src", "source-value-(.*)")`,
	}

	for _, test := range inputs {
		b.Run(readable(test), func(b *testing.B) {
			expr, err := ParseExpr(test)
			require.NoError(b, err)
			for b.Loop() {
				_ = expr.String()
			}
		})
	}
}

func TestVectorSelector_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		vs       VectorSelector
		expected string
	}{
		{
			name:     "empty value",
			vs:       VectorSelector{},
			expected: ``,
		},
		{
			name:     "no matchers with name",
			vs:       VectorSelector{Name: "foobar"},
			expected: `foobar`,
		},
		{
			name: "one matcher with name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
				},
			},
			expected: `foobar{a="x"}`,
		},
		{
			name: "two matchers with name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
					labels.MustNewMatcher(labels.MatchEqual, "b", "y"),
				},
			},
			expected: `foobar{a="x",b="y"}`,
		},
		{
			name: "two matchers without name",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
					labels.MustNewMatcher(labels.MatchEqual, "b", "y"),
				},
			},
			expected: `{a="x",b="y"}`,
		},
		{
			name: "name matcher and name",
			vs: VectorSelector{
				Name: "foobar",
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "foobar"),
				},
			},
			expected: `foobar`,
		},
		{
			name: "name matcher only",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "foobar"),
				},
			},
			expected: `{__name__="foobar"}`,
		},
		{
			name: "empty name matcher",
			vs: VectorSelector{
				LabelMatchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, ""),
					labels.MustNewMatcher(labels.MatchEqual, "a", "x"),
				},
			},
			expected: `{__name__="",a="x"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.vs.String())
		})
	}
}

func TestBinaryExprUTF8Labels(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "UTF-8 labels in on clause",
			input:    `foo / on("äää") bar`,
			expected: `foo / on ("äää") bar`,
		},
		{
			name:     "UTF-8 labels in ignoring clause",
			input:    `foo / ignoring("üüü") bar`,
			expected: `foo / ignoring ("üüü") bar`,
		},
		{
			name:     "UTF-8 labels in group_left clause",
			input:    `foo / on("äää") group_left("ööö") bar`,
			expected: `foo / on ("äää") group_left ("ööö") bar`,
		},
		{
			name:     "UTF-8 labels in group_right clause",
			input:    `foo / on("äää") group_right("ööö") bar`,
			expected: `foo / on ("äää") group_right ("ööö") bar`,
		},
		{
			name:     "Mixed legacy and UTF-8 labels",
			input:    `foo / on(legacy, "üüü") bar`,
			expected: `foo / on (legacy, "üüü") bar`,
		},
		{
			name:     "Legacy labels only (should not quote)",
			input:    `foo / on(job, instance) bar`,
			expected: `foo / on (job, instance) bar`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := ParseExpr(tc.input)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}
			result := expr.String()
			if result != tc.expected {
				t.Errorf("Expected: %s\nGot: %s", tc.expected, result)
			}
		})
	}
}
