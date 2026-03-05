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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAggregateExprPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `sum(foo)`,
			out: `sum(foo)`,
		},
		{
			in: `sum by() (task:errors:rate10s{job="s"})`,
			out: `sum(
  task:errors:rate10s{job="s"}
)`,
		},
		{
			in: `sum without(job,foo) (task:errors:rate10s{job="s"})`,
			out: `sum without (job, foo) (
  task:errors:rate10s{job="s"}
)`,
		},
		{
			in: `sum(task:errors:rate10s{job="s"}) without(job,foo)`,
			out: `sum without (job, foo) (
  task:errors:rate10s{job="s"}
)`,
		},
		{
			in: `sum by(job,foo) (task:errors:rate10s{job="s"})`,
			out: `sum by (job, foo) (
  task:errors:rate10s{job="s"}
)`,
		},
		{
			in: `sum (task:errors:rate10s{job="s"}) by(job,foo)`,
			out: `sum by (job, foo) (
  task:errors:rate10s{job="s"}
)`,
		},
		{
			in: `topk(10, ask:errors:rate10s{job="s"})`,
			out: `topk(
  10,
  ask:errors:rate10s{job="s"}
)`,
		},
		{
			in: `sum by(job,foo) (sum by(job,foo) (task:errors:rate10s{job="s"}))`,
			out: `sum by (job, foo) (
  sum by (job, foo) (
    task:errors:rate10s{job="s"}
  )
)`,
		},
		{
			in: `sum by(job,foo) (sum by(job,foo) (sum by(job,foo) (task:errors:rate10s{job="s"})))`,
			out: `sum by (job, foo) (
  sum by (job, foo) (
    sum by (job, foo) (
      task:errors:rate10s{job="s"}
    )
  )
)`,
		},
		{
			in: `sum by(job,foo)
(sum by(job,foo) (task:errors:rate10s{job="s"}))`,
			out: `sum by (job, foo) (
  sum by (job, foo) (
    task:errors:rate10s{job="s"}
  )
)`,
		},
		{
			in: `sum by(job,foo)
(sum(task:errors:rate10s{job="s"}) without(job,foo))`,
			out: `sum by (job, foo) (
  sum without (job, foo) (
    task:errors:rate10s{job="s"}
  )
)`,
		},
		{
			in: `sum by(job,foo) # Comment 1.
(sum by(job,foo) ( # Comment 2.
task:errors:rate10s{job="s"}))`,
			out: `sum by (job, foo) (
  sum by (job, foo) (
    task:errors:rate10s{job="s"}
  )
)`,
		},
	}
	for _, test := range inputs {
		expr, err := testParser.ParseExpr(test.in)
		require.NoError(t, err)

		require.Equal(t, test.out, Prettify(expr))
	}
}

func TestBinaryExprPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `a+b`,
			out: `a + b`,
		},
		{
			in: `a == bool 1`,
			out: `  a
== bool
  1`,
		},
		{
			in: `a + ignoring(job) b`,
			out: `  a
+ ignoring (job)
  b`,
		},
		{
			in: `foo_1 + foo_2`,
			out: `  foo_1
+
  foo_2`,
		},
		{
			in: `foo_1 + foo_2 + foo_3`,
			out: `    foo_1
  +
    foo_2
+
  foo_3`,
		},
		{
			in: `foo + baar + foo_3`,
			out: `  foo + baar
+
  foo_3`,
		},
		{
			in: `foo_1 + foo_2 + foo_3 + foo_4`,
			out: `      foo_1
    +
      foo_2
  +
    foo_3
+
  foo_4`,
		},
		{
			in: `foo_1 + ignoring(foo) foo_2 + ignoring(job) group_left foo_3 + on(instance) group_right foo_4`,
			out: `      foo_1
    + ignoring (foo)
      foo_2
  + ignoring (job) group_left ()
    foo_3
+ on (instance) group_right ()
  foo_4`,
		},
	}
	for _, test := range inputs {
		t.Run(test.in, func(t *testing.T) {
			expr, err := testParser.ParseExpr(test.in)
			require.NoError(t, err)

			require.Equal(t, test.out, Prettify(expr))
		})
	}
}

func TestCallExprPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in: `rate(foo[1m])`,
			out: `rate(
  foo[1m]
)`,
		},
		{
			in: `sum_over_time(foo[1m])`,
			out: `sum_over_time(
  foo[1m]
)`,
		},
		{
			in: `rate(long_vector_selector[10m:1m] @ start() offset 1m)`,
			out: `rate(
  long_vector_selector[10m:1m] @ start() offset 1m
)`,
		},
		{
			in: `histogram_quantile(0.9, rate(foo[1m]))`,
			out: `histogram_quantile(
  0.9,
  rate(
    foo[1m]
  )
)`,
		},
		{
			in: `max_over_time(rate(demo_api_request_duration_seconds_count[1m])[1m:] @ start() offset 1m)`,
			out: `max_over_time(
  rate(
    demo_api_request_duration_seconds_count[1m]
  )[1m:] @ start() offset 1m
)`,
		},
		{
			in: `label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")`,
			out: `label_replace(
  up{job="api-server",service="a:c"},
  "foo",
  "$1",
  "service",
  "(.*):.*"
)`,
		},
		{
			in: `label_replace(label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*"), "foo", "$1", "service", "(.*):.*")`,
			out: `label_replace(
  label_replace(
    up{job="api-server",service="a:c"},
    "foo",
    "$1",
    "service",
    "(.*):.*"
  ),
  "foo",
  "$1",
  "service",
  "(.*):.*"
)`,
		},
	}
	for _, test := range inputs {
		expr, err := testParser.ParseExpr(test.in)
		require.NoError(t, err)

		fmt.Println("=>", expr.String())
		require.Equal(t, test.out, Prettify(expr))
	}
}

func TestParenExprPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `(foo)`,
			out: `(foo)`,
		},
		{
			in: `(_foo_long_)`,
			out: `(
  _foo_long_
)`,
		},
		{
			in: `((foo_long))`,
			out: `(
  (foo_long)
)`,
		},
		{
			in: `((_foo_long_))`,
			out: `(
  (
    _foo_long_
  )
)`,
		},
		{
			in: `(((foo_long)))`,
			out: `(
  (
    (foo_long)
  )
)`,
		},
	}
	for _, test := range inputs {
		expr, err := testParser.ParseExpr(test.in)
		require.NoError(t, err)

		require.Equal(t, test.out, Prettify(expr))
	}
}

func TestStepInvariantExpr(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `a @ 1`,
			out: `a @ 1.000`,
		},
		{
			in:  `a @ start()`,
			out: `a @ start()`,
		},
		{
			in:  `vector_selector @ start()`,
			out: `vector_selector @ start()`,
		},
	}
	for _, test := range inputs {
		expr, err := testParser.ParseExpr(test.in)
		require.NoError(t, err)

		require.Equal(t, test.out, Prettify(expr))
	}
}

func TestExprPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `(1 + 2)`,
			out: `(1 + 2)`,
		},
		{
			in: `(foo + bar)`,
			out: `(
  foo + bar
)`,
		},
		{
			in: `(foo_long + bar_long)`,
			out: `(
    foo_long
  +
    bar_long
)`,
		},
		{
			in: `(foo_long + bar_long + bar_2_long)`,
			out: `(
      foo_long
    +
      bar_long
  +
    bar_2_long
)`,
		},
		{
			in: `((foo_long + bar_long) + bar_2_long)`,
			out: `(
    (
        foo_long
      +
        bar_long
    )
  +
    bar_2_long
)`,
		},
		{
			in: `(1111 + 2222)`,
			out: `(
    1111
  +
    2222
)`,
		},
		{
			in: `(sum_over_time(foo[1m]))`,
			out: `(
  sum_over_time(
    foo[1m]
  )
)`,
		},
		{
			in: `histogram_quantile(0.9, rate(foo[1m] @ start()))`,
			out: `histogram_quantile(
  0.9,
  rate(
    foo[1m] @ start()
  )
)`,
		},
		{
			in: `(label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*"))`,
			out: `(
  label_replace(
    up{job="api-server",service="a:c"},
    "foo",
    "$1",
    "service",
    "(.*):.*"
  )
)`,
		},
		{
			in: `(label_replace(label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*"), "foo", "$1", "service", "(.*):.*"))`,
			out: `(
  label_replace(
    label_replace(
      up{job="api-server",service="a:c"},
      "foo",
      "$1",
      "service",
      "(.*):.*"
    ),
    "foo",
    "$1",
    "service",
    "(.*):.*"
  )
)`,
		},
		{
			in: `(label_replace(label_replace((up{job="api-server",service="a:c"}), "foo", "$1", "service", "(.*):.*"), "foo", "$1", "service", "(.*):.*"))`,
			out: `(
  label_replace(
    label_replace(
      (
        up{job="api-server",service="a:c"}
      ),
      "foo",
      "$1",
      "service",
      "(.*):.*"
    ),
    "foo",
    "$1",
    "service",
    "(.*):.*"
  )
)`,
		},
		// Following queries have been taken from https://monitoring.mixins.dev/
		{
			in: `(node_filesystem_avail_bytes{job="node",fstype!=""} / node_filesystem_size_bytes{job="node",fstype!=""} * 100 < 40 and predict_linear(node_filesystem_avail_bytes{job="node",fstype!=""}[6h], 24*60*60) < 0 and node_filesystem_readonly{job="node",fstype!=""} == 0)`,
			out: `(
            node_filesystem_avail_bytes{fstype!="",job="node"}
          /
            node_filesystem_size_bytes{fstype!="",job="node"}
        *
          100
      <
        40
    and
        predict_linear(
          node_filesystem_avail_bytes{fstype!="",job="node"}[6h],
            24 * 60
          *
            60
        )
      <
        0
  and
      node_filesystem_readonly{fstype!="",job="node"}
    ==
      0
)`,
		},
		{
			in: `(node_filesystem_avail_bytes{job="node",fstype!=""} / node_filesystem_size_bytes{job="node",fstype!=""} * 100 < 20 and predict_linear(node_filesystem_avail_bytes{job="node",fstype!=""}[6h], 4*60*60) < 0 and node_filesystem_readonly{job="node",fstype!=""} == 0)`,
			out: `(
            node_filesystem_avail_bytes{fstype!="",job="node"}
          /
            node_filesystem_size_bytes{fstype!="",job="node"}
        *
          100
      <
        20
    and
        predict_linear(
          node_filesystem_avail_bytes{fstype!="",job="node"}[6h],
            4 * 60
          *
            60
        )
      <
        0
  and
      node_filesystem_readonly{fstype!="",job="node"}
    ==
      0
)`,
		},
		{
			in: `(node_timex_offset_seconds > 0.05 and deriv(node_timex_offset_seconds[5m]) >= 0) or (node_timex_offset_seconds < -0.05 and deriv(node_timex_offset_seconds[5m]) <= 0)`,
			out: `  (
        node_timex_offset_seconds
      >
        0.05
    and
        deriv(
          node_timex_offset_seconds[5m]
        )
      >=
        0
  )
or
  (
        node_timex_offset_seconds
      <
        -0.05
    and
        deriv(
          node_timex_offset_seconds[5m]
        )
      <=
        0
  )`,
		},
		{
			in: `1 - ((node_memory_MemAvailable_bytes{job="node"} or (node_memory_Buffers_bytes{job="node"} + node_memory_Cached_bytes{job="node"} + node_memory_MemFree_bytes{job="node"} + node_memory_Slab_bytes{job="node"}) ) / node_memory_MemTotal_bytes{job="node"})`,
			out: `  1
-
  (
      (
          node_memory_MemAvailable_bytes{job="node"}
        or
          (
                  node_memory_Buffers_bytes{job="node"}
                +
                  node_memory_Cached_bytes{job="node"}
              +
                node_memory_MemFree_bytes{job="node"}
            +
              node_memory_Slab_bytes{job="node"}
          )
      )
    /
      node_memory_MemTotal_bytes{job="node"}
  )`,
		},
		{
			in: `min by (job, integration) (rate(alertmanager_notifications_failed_total{job="alertmanager", integration=~".*"}[5m]) / rate(alertmanager_notifications_total{job="alertmanager", integration="~.*"}[5m])) > 0.01`,
			out: `  min by (job, integration) (
      rate(
        alertmanager_notifications_failed_total{integration=~".*",job="alertmanager"}[5m]
      )
    /
      rate(
        alertmanager_notifications_total{integration="~.*",job="alertmanager"}[5m]
      )
  )
>
  0.01`,
		},
		{
			in: `(count by (job) (changes(process_start_time_seconds{job="alertmanager"}[10m]) > 4) / count by (job) (up{job="alertmanager"})) >= 0.5`,
			out: `  (
      count by (job) (
          changes(
            process_start_time_seconds{job="alertmanager"}[10m]
          )
        >
          4
      )
    /
      count by (job) (
        up{job="alertmanager"}
      )
  )
>=
  0.5`,
		},
	}
	for _, test := range inputs {
		expr, err := testParser.ParseExpr(test.in)
		require.NoError(t, err)
		require.Equal(t, test.out, Prettify(expr))
	}
}

func TestUnaryPretty(t *testing.T) {
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in:  `-1`,
			out: `-1`,
		},
		{
			in:  `-vector_selector`,
			out: `-vector_selector`,
		},
		{
			in: `(-vector_selector)`,
			out: `(
  -vector_selector
)`,
		},
		{
			in: `-histogram_quantile(0.9,rate(foo[1m]))`,
			out: `-histogram_quantile(
  0.9,
  rate(
    foo[1m]
  )
)`,
		},
		{
			in: `-histogram_quantile(0.99, sum by (le) (rate(foo[1m])))`,
			out: `-histogram_quantile(
  0.99,
  sum by (le) (
    rate(
      foo[1m]
    )
  )
)`,
		},
		{
			in: `-histogram_quantile(0.9, -rate(foo[1m] @ start()))`,
			out: `-histogram_quantile(
  0.9,
  -rate(
    foo[1m] @ start()
  )
)`,
		},
		{
			in: `(-histogram_quantile(0.9, -rate(foo[1m] @ start())))`,
			out: `(
  -histogram_quantile(
    0.9,
    -rate(
      foo[1m] @ start()
    )
  )
)`,
		},
	}
	for _, test := range inputs {
		t.Run(test.in, func(t *testing.T) {
			expr, err := testParser.ParseExpr(test.in)
			require.NoError(t, err)
			require.Equal(t, test.out, Prettify(expr))
		})
	}
}

func TestDurationExprPretty(t *testing.T) {
	optsParser := NewParser(Options{ExperimentalDurationExpr: true})
	maxCharactersPerLine = 10
	inputs := []struct {
		in, out string
	}{
		{
			in: `rate(foo[2*1h])`,
			out: `rate(
  foo[2 * 1h]
)`,
		},
		{
			in: `rate(foo[2*1h])`,
			out: `rate(
  foo[2 * 1h]
)`,
		},
		{
			in: `rate(foo[-5m+35m])`,
			out: `rate(
  foo[-5m + 35m]
)`,
		},
	}
	for _, test := range inputs {
		t.Run(test.in, func(t *testing.T) {
			expr, err := optsParser.ParseExpr(test.in)
			require.NoError(t, err)
			require.Equal(t, test.out, Prettify(expr))
		})
	}
}
