// Copyright 2020 The Prometheus Authors
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

package prettier

import (
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

type prettierTest struct {
	expr     string
	expected string
}

var sortingLexItemCases = []prettierTest{
	{
		expr:     `sum(go_alloc_bytes) by (job)`,
		expected: `sum by (job) (go_alloc_bytes)`,
	},
	{
		expr:     `sum by (job) (go_alloc_bytes)`,
		expected: `sum by(job) (go_alloc_bytes)`,
	},
	{
		expr:     `metric_first + metric_second`,
		expected: `metric_first + metric_second`,
	},
	{
		expr:     `metric_first + bool metric_second`,
		expected: `metric_first + bool metric_second`,
	},
	{
		expr:     `metric_first + ignoring(job) metric_second`,
		expected: `metric_first + ignoring(job) metric_second`,
	},
	{
		expr:     `sum without(job, instance, foo) (metric_first + metric_second)`,
		expected: `sum without(job, instance, foo) (metric_first + metric_second)`,
	},
	{
		expr:     `quantile(0.9, sum(go_goroutines) without(job)) by (instance)`,
		expected: `quantile by (instance) (0.9, sum without(job) (go_goroutines))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) without(job)) by (instance)`,
		expected: `quantile by (instance) (0.9, sum without(job) (min by (job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) without(job)) by (instance)`,
		expected: `quantile by(instance) (0.9, sum without(job) (min by(job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) ignoring(job)) by (instance)`,
		expected: `quantile by (instance) (0.9, sum ignoring(job) (min by (job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) ignoring(job)) by (instance)`,
		expected: `quantile by (instance) (0.9, sum ignoring(job) (min by (job) (go_alloc_bytes)))`,
	},
	{
		expr:     `sum(metric_first + metric_second) without(job)`,
		expected: `sum without(job) (metric_first + metric_second)`,
	},
	{
		expr:     `sum(metric_first + metric_second) without(job, instance, foo)`,
		expected: `sum without(job, instance, foo) (metric_first + metric_second)`,
	},
	{
		expr:     `min(sum(metric_first + metric_second) without(job, instance, foo)) by(job)`,
		expected: `min by(job) (sum without(job, instance, foo) (metric_first + metric_second))`,
	},
	{
		expr:     `min by(job) (sum(metric_first + metric_second) without(job, instance, foo))`,
		expected: `min by(job) (sum without(job, instance, foo) (metric_first + metric_second))`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum(metric_first + metric_second) without(job, instance, foo)))`,
		expected: `quantile(0.9, min by(job) (sum without(job, instance, foo) (metric_first + metric_second)))`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum(metric_first + metric_second) without(job, instance, foo))) ignoring(foo)`,
		expected: `quantile ignoring(foo) (0.9, min by(job) (sum without(job, instance, foo) (metric_first + metric_second)))`,
	},
	{
		expr:     `sum(go_alloc_bytes{job="prometheus", instance="localhost:9090"}) by (job)`,
		expected: `sum by (job) (go_alloc_bytes{job="prometheus", instance="localhost:9090"})`,
	},
	{
		expr:     `sum(go_alloc_bytes{job="prometheus", instance="localhost:9090"} + ignoring(instance) go_goroutines{job="prometheus", instance="localhost:9090"}) by (job)`,
		expected: `sum by (job) (go_alloc_bytes{job="prometheus", instance="localhost:9090"} + ignoring(instance) go_goroutines{job="prometheus", instance="localhost:9090"})`,
	},
	// __name__ cases
	{
		expr:     `{__name__="metric_name"}`,
		expected: `metric_name`,
	},
	{
		expr:     `{__name__="metric_name",}`,
		expected: `metric_name`,
	},
	{
		expr:     `{__name__="metric_name", foo="bar"}`,
		expected: `metric_name{foo="bar"}`,
	},
	{
		expr:     `{__name__="metric_name", foo="bar", first="second"}`,
		expected: `metric_name{foo="bar", first="second"}`,
	},
	{
		expr:     `{foo="bar", __name__="metric_name"}`,
		expected: `metric_name{foo="bar",}`,
	},
	{
		expr:     `{__name__="metric_first", foo="bar"} + {__name__="metric_second", foo="bar"} + {__name__="metric_third", foo="bar"}`,
		expected: `metric_first{foo="bar"} + metric_second{foo="bar"} + metric_third{foo="bar"}`,
	},
	{
		expr:     `{foo="bar", __name__="metric_name", first="second"}`,
		expected: `metric_name{foo="bar", first="second"}`,
	},
	{
		expr:     `{foo="bar", first="second", __name__="metric_name"}`,
		expected: `metric_name{foo="bar", first="second",}`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum({__name__="metric_first"} + metric_second) without(job, instance, foo)))`,
		expected: `quantile(0.9, min by(job) (sum without(job, instance, foo) (metric_first + metric_second)))`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum(metric_first + {__name__="metric_second"}) without(job, instance, foo))) ignoring(foo)`,
		expected: `quantile ignoring(foo) (0.9, min by(job) (sum without(job, instance, foo) (metric_first + metric_second)))`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum({__name__="metric_first"} + {__name__="metric_second"}) without(job, instance, foo))) ignoring(foo)`,
		expected: `quantile ignoring(foo) (0.9, min by(job) (sum without(job, instance, foo) (metric_first + metric_second)))`,
	},
	{
		expr:     `quantile(0.9, min by(job) (sum({__name__="metric_first", instance="first"} + {__name__="metric_second", instance="second"}) without(job, instance, foo))) ignoring(foo)`,
		expected: `quantile ignoring(foo) (0.9, min by(job) (sum without(job, instance, foo) (metric_first{instance="first"} + metric_second{instance="second"})))`,
	},
	{
		expr:     `sum({__name__="metric_first", instance="first"} + {__name__="metric_second", instance="second"}) without(job, instance, foo)`,
		expected: `sum without(job, instance, foo) (metric_first{instance="first"} + metric_second{instance="second"})`,
	},
	{
		expr:     `sum({instance="first", __name__="metric_first"} + {instance="second", __name__="metric_second"}) without(job, instance, foo)`,
		expected: `sum without(job, instance, foo) (metric_first{instance="first",} + metric_second{instance="second",})`,
	},
	{
		expr:     `sum({instance="first", __name__="metric_first", foo="bar"} + {instance="second", __name__="metric_second", foo="bar"}) without(job, instance, foo)`,
		expected: `sum without(job, instance, foo) (metric_first{instance="first", foo="bar"} + metric_second{instance="second", foo="bar"})`,
	},
	// with comments
	{
		expr: `sum # comment
(go_alloc_bytes) by (job)`,
		expected: `sum by (job) # comment
(go_alloc_bytes)`,
	},
	{
		expr: `sum # comment
(go_alloc_bytes) by # comment
(job)`,
		expected: `sum by # comment
(job) # comment
(go_alloc_bytes)`,
	},
	{
		expr: `sum (go_alloc_bytes) # comment
by (job)`,
		expected: `sum by (job) (go_alloc_bytes) # comment`,
	},
	{
		expr: `sum ( # comment
go_alloc_bytes # comment
) # comment
by (job)`,
		expected: `sum by (job) ( # comment
go_alloc_bytes # comment
) # comment`,
	},
	{
		expr: `sum (go_alloc_bytes) # comment
by ( # comment
job # comment
) # comment`,
		expected: `sum by ( # comment
job # comment
) (go_alloc_bytes) # comment
# comment`,
	},
	{
		expr: `# comment
{__name__="metric_name"}`,
		expected: `# comment
metric_name`,
	},
	{
		expr: `{ # comment
__name__="metric_name"}`,
		expected: `metric_name{ # comment
}`,
	},
	// We should not format those cases that contain comments within __name__ label-value.
	{
		expr: `{__name__ # comment
="metric_name"}`,
		expected: `{__name__ # comment
="metric_name"}`,
	},
	{
		expr: `{__name__= # comment
"metric_name"}`,
		expected: `{__name__= # comment
"metric_name"}`,
	},
	{
		expr: `{__name__="metric_name" # comment
}`,
		expected: `metric_name{ # comment
}`,
	},
	{
		expr:     `{__name__="metric_name"} # comment`,
		expected: `metric_name # comment`,
	},
}

func TestLexItemSorting(t *testing.T) {
	prettier := New("", 100, IndentAsSpace)
	for i, expr := range sortingLexItemCases {
		expectedSlice := prettier.refreshLexItems(prettier.lexItems(expr.expected))
		input := prettier.lexItems(expr.expr)
		testutil.Equals(t, expectedSlice, prettier.sortItems(input), "%d: input %q", i, expr.expr)
	}
}

var prettierCases = []prettierTest{
	{
		expr:     `go_goroutines`,
		expected: `go_goroutines`,
	},
	{
		expr:     `go_goroutines{job="prometheus", instance="localhost:9090"}`,
		expected: `go_goroutines{job="prometheus", instance="localhost:9090"}`,
	},
	{
		expr:     `go_goroutines{job="prometheus",instance="localhost:9090"}`,
		expected: `go_goroutines{job="prometheus", instance="localhost:9090"}`,
	},
	{
		expr: `instance_cpu_time_ns{app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost"}`,
		expected: `  instance_cpu_time_ns{
    app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost",
  }`,
	},
	{
		expr: `instance_cpu_time_ns{app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost",}`,
		expected: `  instance_cpu_time_ns{
    app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost",
  }`,
	},
	{
		expr: `metric_one + metric_two`,
		expected: `    metric_one
  +
    metric_two`,
	},
	{
		expr: `metric_one <= metric_two`,
		expected: `    metric_one
  <=
    metric_two`,
	},
	{
		expr: `metric_one{foo="bar"} + metric_two{foo="bar"}`,
		expected: `    metric_one{foo="bar"}
  +
    metric_two{foo="bar"}`,
	},
	{
		expr: `metric_one{foo="bar"} + metric_two{foo="bar", instance="localhost:31233", job="two", first="second_", job="cluster-manager"}`,
		expected: `    metric_one{foo="bar"}
  +
    metric_two{foo="bar", instance="localhost:31233", job="two", first="second_", job="cluster-manager"}`,
	},
	{
		expr: `metric_two{foo="bar", instance="localhost:31233", job="two", first="second_", job="cluster-manager"} + metric_one{foo="bar"}`,
		expected: `    metric_two{foo="bar", instance="localhost:31233", job="two", first="second_", job="cluster-manager"}
  +
    metric_one{foo="bar"}`,
	},
	{
		expr: `metric_one + metric_two + metric_three{some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    metric_one
  +
    metric_two
  +
    metric_three{some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}`,
	},
	{
		expr: `metric_one + metric_two + metric_three{a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    metric_one
  +
    metric_two
  +
    metric_three{
      a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value",
    }`,
	},
	{
		expr: `metric_one + metric_two + metric_three + metric_four{_a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    metric_one
  +
    metric_two
  +
    metric_three
  +
    metric_four{
      _a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value",
    }`,
	},
	{
		expr: `metric_one + metric_two + metric_three + metric_four{_a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"} + metric_five`,
		expected: `    metric_one
  +
    metric_two
  +
    metric_three
  +
    metric_four{
      _a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value",
    }
  +
    metric_five`,
	},
	{
		expr: `metric_one + ignoring(job) metric_two`,
		expected: `    metric_one
  + ignoring(job)
    metric_two`,
	},
	{
		expr: `metric_one + ignoring(job) metric_two - ignoring(job) metric_three`,
		expected: `    metric_one
  + ignoring(job)
    metric_two
  - ignoring(job)
    metric_three`,
	},
	{
		expr: `metric_one + ignoring(job) metric_two - ignoring(job) metric_three{a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    metric_one
  + ignoring(job)
    metric_two
  - ignoring(job)
    metric_three{
      a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value",
    }`,
	},
	{
		expr: `metric_one - ignoring(job) metric_two{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"} + ignoring(job) metric_three`,
		expected: `    metric_one
  - ignoring(job)
    metric_two{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }
  + ignoring(job)
    metric_three`,
	},
	{
		expr: `metric_one <= bool metric_two`,
		expected: `    metric_one
  <= bool
    metric_two`,
	},
	{
		expr:     `(metric_name)`,
		expected: `(metric_name)`,
	},
	{
		expr:     `((metric_name))`,
		expected: `((metric_name))`,
	},
	{
		expr: `(metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"})`,
		expected: `  (
    metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }
  )`,
	},
	{
		expr: `1 + metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    1 + metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }`,
	},
	{
		expr: `metric_one + 2 + metric_two{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}`,
		expected: `    metric_one + 2
  +
    metric_two{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }`,
	},
	{
		expr: `1 + metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"} + 2`,
		expected: `    1 + metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    } + 2`,
	},
	{
		expr: `metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"} + 1`,
		expected: `    metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    } + 1`,
	},
	{
		expr:     `metric_one[5m]`,
		expected: `metric_one[5m]`,
	},
	{
		expr: `metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m]`,
		expected: `  metric_one{
    _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
  }[5m]`,
	},
	{
		expr:     `metric_one[5m:1m]`,
		expected: `metric_one[5m:1m]`,
	},
	{
		expr: `metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m:1m]`,
		expected: `  metric_one{
    _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
  }[5m:1m]`,
	},
	{
		expr: `(metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m])`,
		expected: `  (
    metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }[5m]
  )`,
	},
	{
		expr: `((metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m]))`,
		expected: `  (
    (
      metric_one{
        _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
      }[5m]
    )
  )`,
	},
	{
		expr: `(metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m:1m])`,
		expected: `  (
    metric_one{
      _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
    }[5m:1m]
  )`,
	},
	{
		expr: `((metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m:1m]))`,
		expected: `  (
    (
      metric_one{
        _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
      }[5m:1m]
    )
  )`,
	},
	{
		expr: `((((metric_one{_a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value"}[5m:1m]))))`,
		expected: `  (
    (
      (
        (
          metric_one{
            _a_some_very_large_label="_a_very_large_value", some_very_large_label="a_very_large_value",
          }[5m:1m]
        )
      )
    )
  )`,
	},
	{
		expr:     `rate(metric_name[5m])`,
		expected: `rate(metric_name[5m])`,
	},
	{
		expr: `rate(metric_three{a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value"}[5m])`,
		expected: `  rate(
    metric_three{
      a_some_very_large_label="a_very_large_value", some_very_large_label="a_very_large_value",
    }[5m]
  )`,
	},
	{
		expr: `histogram_quantile(0.9, rate(http_request_duration_seconds_bucket[10m]))`,
		expected: `  histogram_quantile(
    0.9,
    rate(http_request_duration_seconds_bucket[10m])
  )`,
	},
	{
		expr: `label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3")`,
		expected: `  label_join(
    up{job="api-server", src1="a", src2="b", src3="c"},
    "foo",
    ",",
    "src1",
    "src2",
    "src3"
  )`,
	},
	{
		expr: `label_join(label_join(label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3"), "foo", ",", "src1", "src2", "src3"), "foo", ",", "src1", "src2", "src3")`,
		expected: `  label_join(
    label_join(
      label_join(
        up{job="api-server", src1="a", src2="b", src3="c"},
        "foo",
        ",",
        "src1",
        "src2",
        "src3"
      ),
      "foo",
      ",",
      "src1",
      "src2",
      "src3"
    ),
    "foo",
    ",",
    "src1",
    "src2",
    "src3"
  )`,
	},
	{
		expr: `a + label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3")`,
		expected: `    a
  +
    label_join(
      up{job="api-server", src1="a", src2="b", src3="c"},
      "foo",
      ",",
      "src1",
      "src2",
      "src3"
    )`,
	},
	{
		expr: `label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3") + label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")`,
		expected: `    label_join(
      up{job="api-server", src1="a", src2="b", src3="c"},
      "foo",
      ",",
      "src1",
      "src2",
      "src3"
    )
  +
    label_replace(
      up{job="api-server", service="a:c"},
      "foo",
      "$1",
      "service",
      "(.*):.*"
    )`,
	},
	{
		expr: `(label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3") + label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*"))`,
		expected: `  (
      label_join(
        up{job="api-server", src1="a", src2="b", src3="c"},
        "foo",
        ",",
        "src1",
        "src2",
        "src3"
      )
    +
      label_replace(
        up{job="api-server", service="a:c"},
        "foo",
        "$1",
        "service",
        "(.*):.*"
      )
  )`,
	},
	{
		expr: `((label_join(up{job="api-server",src1="a",src2="b",src3="c"}, "foo", ",", "src1", "src2", "src3") + label_replace(up{job="api-server",service="a:c"}, "foo", "$1", "service", "(.*):.*")))`,
		expected: `  (
    (
        label_join(
          up{job="api-server", src1="a", src2="b", src3="c"},
          "foo",
          ",",
          "src1",
          "src2",
          "src3"
        )
      +
        label_replace(
          up{job="api-server", service="a:c"},
          "foo",
          "$1",
          "service",
          "(.*):.*"
        )
    )
  )`,
	},
	{
		expr: `label_join((up), "foo", ",", "src1", "src2", "src3")`,
		expected: `  label_join(
    (up),
    "foo",
    ",",
    "src1",
    "src2",
    "src3"
  )`,
	},
	{
		expr:     `sum(metric_name)`,
		expected: `sum(metric_name)`,
	},
	{
		expr:     `sum without(label) (metric_name)`,
		expected: `sum without(label) (metric_name)`,
	},
	{
		expr:     `sum (metric_name) without(label)`,
		expected: `sum without(label) (metric_name)`,
	},
	{
		expr: `sum without(label) (metric_three{a_some_very_large_label="a_very_large_value", label="a_very_large_value"})`,
		expected: `  sum without(label) (
    metric_three{a_some_very_large_label="a_very_large_value", label="a_very_large_value"}
  )`,
	},
	{
		expr: `sum (metric_three{a_some_very_large_label="a_very_large_value", a_some_very_large_label_2="a_very_large_value"}) without(label)`,
		expected: `  sum without(label) (
    metric_three{
      a_some_very_large_label="a_very_large_value", a_some_very_large_label_2="a_very_large_value",
    }
  )`,
	},
	{
		expr: `sum (metric_three{a_some_very_large_label="a_very_large_value", label="a_very_large_value"}) without(label)`,
		expected: `  sum without(label) (
    metric_three{a_some_very_large_label="a_very_large_value", label="a_very_large_value"}
  )`,
	},
	// Comments.
	{
		expr:     `metric_name # comment`,
		expected: `metric_name # comment`,
	},
	{
		expr: `metric_name # comment 1
          # comment 2`,
		expected: `  metric_name # comment 1
 # comment 2`,
	},
	{
		expr: `metric_three{ # comment 1
          a_some_very_large_label="a_very_large_value", # comment 2
          a_some_very_large_label_2="a_very_large_value"}`,
		expected: `  metric_three{
  # comment 1
    a_some_very_large_label="a_very_large_value", # comment 2
   a_some_very_large_label_2="a_very_large_value",
  }`,
	},
	{
		expr: `metric_three # comment
          {
          a_some_very_large_label=  # comment
          "a_very_large_value",
          a_some_very_large_label_2="a_very_large_value"}`,
		expected: `  metric_three # comment
  {
    a_some_very_large_label= # comment
  "a_very_large_value", a_some_very_large_label_2="a_very_large_value",
  }`,
	},
	// examples from real-world.
	{
		expr: `rate(demo_api_request_duration_seconds_count{status="500",job="demo"}[5m]) * 50 > on(job, instance, method, path) rate(demo_api_request_duration_seconds_count{status="200",job="demo"}[5m])`,
		expected: `    rate(demo_api_request_duration_seconds_count{status="500", job="demo"}[5m]) * 50
  > on(job, instance, method, path)
    rate(demo_api_request_duration_seconds_count{status="200", job="demo"}[5m])`,
	},
	{
		expr: `histogram_quantile(0.9, rate(demo_api_request_duration_seconds_bucket{job="demo"}[5m])) > 0.05 and rate(demo_api_request_duration_seconds_count{job="demo"}[5m]) > 1`,
		expected: `    histogram_quantile(
      0.9,
      rate(demo_api_request_duration_seconds_bucket{job="demo"}[5m])
    ) > 0.05
  and
    rate(demo_api_request_duration_seconds_count{job="demo"}[5m]) > 1`,
	},
	{
		expr: `sum (container_memory_working_set_bytes{pod_name=~"^$Deployment$Statefulset$Daemonset.*$", kubernetes_io_hostname=~"^$Node$", pod_name!=""})`,
		expected: `  sum(
    container_memory_working_set_bytes{
      pod_name=~"^$Deployment$Statefulset$Daemonset.*$", kubernetes_io_hostname=~"^$Node$", pod_name!="",
    }
  )`,
	},
	{
		expr:     `sort_desc(sum(sum_over_time(ALERTS{alertstate="firing"}[24h])) by (alertname))`,
		expected: `sort_desc(sum by(alertname) (sum_over_time(ALERTS{alertstate="firing"}[24h])))`,
	},
	{
		expr: `(sum(node_memory_MemTotal) - sum(node_memory_MemFree + node_memory_Buffers + node_memory_Cached) ) / sum(node_memory_MemTotal) * 100 > 85`,
		expected: `    (sum(node_memory_MemTotal)
      -
        sum(node_memory_MemFree
          +
            node_memory_Buffers
          +
            node_memory_Cached))
  /
    sum(node_memory_MemTotal) * 100 > 85`,
	},
	{
		expr: `100 - ((node_filesystem_avail_bytes{instance=~"$node",mountpoint="$maxmount",fstype=~"ext4|xfs"} * 100) / node_filesystem_size_bytes {instance=~"$node",mountpoint="$maxmount",fstype=~"ext4|xfs"})`,
		expected: `    100 - (
        (node_filesystem_avail_bytes{instance=~"$node", mountpoint="$maxmount", fstype=~"ext4|xfs"} * 100)
      /
        node_filesystem_size_bytes{instance=~"$node", mountpoint="$maxmount", fstype=~"ext4|xfs"}
    )`,
	},
	{
		expr: `(1 - (node_memory_MemAvailable_bytes{instance=~"$node"} / (node_memory_MemTotal_bytes{instance=~"$node"})))* 100`,
		expected: `    (
        1 - (
            node_memory_MemAvailable_bytes{instance=~"$node"}
          /
            (node_memory_MemTotal_bytes{instance=~"$node"})
        )
    ) * 100`,
	},
	{
		expr:     `sum(time() - node_boot_time_seconds{instance=~"$node"})`,
		expected: `sum(time() - node_boot_time_seconds{instance=~"$node"})`,
	},
	{
		expr: `irate(node_network_receive_bytes_total{instance=~'$node',device!~'tap.*|veth.*|br.*|docker.*|virbr*|lo*'}[30m])*8`,
		expected: `    irate(
      node_network_receive_bytes_total{instance=~'$node', device!~'tap.*|veth.*|br.*|docker.*|virbr*|lo*'}[30m]
    ) * 8`,
	},
	{
		expr: `irate(node_disk_write_time_seconds_total{instance=~"$node"}[30m]) / irate(node_disk_writes_completed_total{instance=~"$node"}[30m])`,
		expected: `    irate(node_disk_write_time_seconds_total{instance=~"$node"}[30m])
  /
    irate(node_disk_writes_completed_total{instance=~"$node"}[30m])`,
	},
	{
		expr: `sum (container_memory_working_set_bytes{pod_name=~"^$Deployment$Statefulset$Daemonset.*$", kubernetes_io_hostname=~"^$Node$", pod_name!=""}) / sum (kube_node_status_allocatable_memory_bytes{node=~"^$Node.*$"}) * 100`,
		expected: `    sum(
      container_memory_working_set_bytes{
        pod_name=~"^$Deployment$Statefulset$Daemonset.*$", kubernetes_io_hostname=~"^$Node$", pod_name!="",
      }
    )
  /
    sum(kube_node_status_allocatable_memory_bytes{node=~"^$Node.*$"}) * 100`,
	},
	{
		expr: `sum (rate (container_cpu_usage_seconds_total{image!="",name=~"^k8s_.*",io_kubernetes_container_name!="POD",pod_name=~"^$Deployment$Statefulset$Daemonset.*$",kubernetes_io_hostname=~"^$Node$"}[1m])) by (pod_name,kubernetes_io_hostname)`,
		expected: `  sum by(pod_name, kubernetes_io_hostname) (
    rate(
      container_cpu_usage_seconds_total{
        image!="", name=~"^k8s_.*", io_kubernetes_container_name!="POD", pod_name=~"^$Deployment$Statefulset$Daemonset.*$", kubernetes_io_hostname=~"^$Node$",
      }[1m]
    )
  )`,
	},
	{
		expr: `(sum(kube_deployment_status_replicas_available{deployment=~".*$Deployment$Statefulset$Daemonset"}) or vector(0)) + (sum(kube_statefulset_status_replicas{statefulset=~".*$Deployment$Statefulset$Daemonset"}) or vector(0)) + (sum(kube_daemonset_status_number_ready{daemonset=~".*$Deployment$Statefulset$Daemonset"}) or vector(0))`,
		expected: `    (
        sum(kube_deployment_status_replicas_available{deployment=~".*$Deployment$Statefulset$Daemonset"})
      or
        vector(0)
    )
  +
    (
        sum(kube_statefulset_status_replicas{statefulset=~".*$Deployment$Statefulset$Daemonset"})
      or
        vector(0)
    )
  +
    (
        sum(kube_daemonset_status_number_ready{daemonset=~".*$Deployment$Statefulset$Daemonset"})
      or
        vector(0)
    )`,
	},
	{
		expr: `label_replace((sum(node_filesystem_size_bytes{fstype=~"ext4|xfs"})by (instance)),"ip","$1","instance","(.*):.*")`,
		expected: `  label_replace(
    (sum by(instance) (node_filesystem_size_bytes{fstype=~"ext4|xfs"})),
    "ip",
    "$1",
    "instance",
    "(.*):.*"
  )`,
	},
	{
		expr: `label_replace((sum(rate(container_network_transmit_bytes_total{name!=""}[1m])) by (instance)),"ip","$1","instance","(.*):.*")`,
		expected: `  label_replace(
    (sum by(instance) (rate(container_network_transmit_bytes_total{name!=""}[1m]))),
    "ip",
    "$1",
    "instance",
    "(.*):.*"
  )`,
	},
	{
		expr: `sum(irate(django_http_requests_total_by_view_transport_method_total{namespace=~"$namespace", app=~"^$app$",view!~"prometheus-django-metrics|healthcheck"}[1m])) by(method, view)`,
		expected: `  sum by(method, view) (
    irate(
      django_http_requests_total_by_view_transport_method_total{
        namespace=~"$namespace", app=~"^$app$", view!~"prometheus-django-metrics|healthcheck",
      }[1m]
    )
  )`,
	},
	{
		expr: `histogram_quantile(0.50, sum(rate(django_http_requests_latency_seconds_by_view_method_bucket{namespace=~"$namespace", app=~"^$app$",view!~"prometheus-django-metrics|healthcheck"}[5m])) by (job, le))`,
		expected: `  histogram_quantile(
    0.50,
    sum by(job, le) (
      rate(
        django_http_requests_latency_seconds_by_view_method_bucket{
          namespace=~"$namespace", app=~"^$app$", view!~"prometheus-django-metrics|healthcheck",
        }[5m]
      )
    )
  )`,
	},
	{
		expr: `time() - process_start_time_seconds{job=~"kubernetes-pods", app=~"$app", instance=~"$instance", namespace=~"$namespace"}`,
		expected: `    time() - process_start_time_seconds{
      job=~"kubernetes-pods", app=~"$app", instance=~"$instance", namespace=~"$namespace",
    }`,
	},
	{
		expr: `time() - (alertmanager_build_info{instance=~"$instance"} * on (instance, cluster) group_left process_start_time_seconds{instance=~"$instance"})`,
		expected: `    time() - (
        alertmanager_build_info{instance=~"$instance"}
      * on(instance, cluster)
        group_leftprocess_start_time_seconds{instance=~"$instance"}
    )`,
	},
	{
		expr:     `sum(increase(alertmanager_notifications_failed_total{instance=~"$instance"}[5m])) by (integration)`,
		expected: `sum by(integration) (increase(alertmanager_notifications_failed_total{instance=~"$instance"}[5m]))`,
	},
	{
		expr: `sum(histogram_quantile(0.9,rate(alertmanager_notification_latency_seconds_bucket{instance=~"$instance"}[5m]))) by (integration)`,
		expected: `  sum by(integration) (
    histogram_quantile(
      0.9,
      rate(alertmanager_notification_latency_seconds_bucket{instance=~"$instance"}[5m])
    )
  )`,
	},
	{
		expr: `sum(rate(http_requests_received_total{instance =~ "$instances", controller =~ "$controllers", code =~ "5\\d\\d|4\\d\\d"}[3m])) by (controller)`,
		expected: `  sum by(controller) (
    rate(
      http_requests_received_total{
        instance=~"$instances", controller=~"$controllers", code=~"5\\d\\d|4\\d\\d",
      }[3m]
    )
  )`,
	},
	{
		expr: `sort_desc( sum(certmanager_certificate_expiration_timestamp_seconds{exported_namespace=~"$Namespace"} - time()) by (name,exported_namespace) )`,
		expected: `  sort_desc(
    sum by(name, exported_namespace) (
        certmanager_certificate_expiration_timestamp_seconds{exported_namespace=~"$Namespace"} - time()
    )
  )`,
	},
	{
		expr: `quantile(0.9, sum(min(go_alloc_bytes) by (job)) without(job)) by (instance)`,
		expected: `  quantile by(instance) (0.9,
    sum without(job) (
      min by(job) (go_alloc_bytes)
    )
  )`,
	},
}

func TestPrettierCases(t *testing.T) {
	for _, expr := range prettierCases {
		output, err := New(expr.expr, 100, IndentAsSpace).Prettify()
		testutil.Ok(t, err)
		testutil.Equals(t, expr.expected, output, "formatting does not match")
	}
}
