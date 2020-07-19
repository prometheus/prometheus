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
		expr:     `quantile(0.9, sum(go_goroutines) without(job)) by (localhost)`,
		expected: `quantile by (localhost) (0.9, sum without(job) (go_goroutines))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) without(job)) by (localhost)`,
		expected: `quantile by (localhost) (0.9, sum without(job) (min by (job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) without(job)) by (localhost)`,
		expected: `quantile by(localhost) (0.9, sum without(job) (min by(job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) ignoring(job)) by (localhost)`,
		expected: `quantile by (localhost) (0.9, sum ignoring(job) (min by (job) (go_alloc_bytes)))`,
	},
	{
		expr:     `quantile(0.9, sum(min(go_alloc_bytes) by (job)) ignoring(job)) by (localhost)`,
		expected: `quantile by (localhost) (0.9, sum ignoring(job) (min by (job) (go_alloc_bytes)))`,
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
}

func TestLexItemSorting(t *testing.T) {
	prettier, err := New(PrettifyExpression, "")
	testutil.Ok(t, err)
	for i, expr := range sortingLexItemCases {
		expectedSlice := prettier.refreshLexItems(prettier.lexItems(expr.expected))
		input := prettier.lexItems(expr.expr)
		testutil.Equals(t, expectedSlice, prettier.sortItems(input, true), "%d: input %q", i, expr.expr)
	}
}

var prettierCases = []prettierTest{
	{
		expr:     `go_goroutines`,
		expected: `  go_goroutines`,
	},
	{
		expr:     `go_goroutines{job="prometheus", instance="localhost:9090"}`,
		expected: `  go_goroutines{job="prometheus", instance="localhost:9090"}`,
	},
	{
		expr:     `go_goroutines{job="prometheus",instance="localhost:9090"}`,
		expected: `  go_goroutines{job="prometheus", instance="localhost:9090"}`,
	},
	{
		expr: `instance_cpu_time_ns{app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost"}`,
		expected: `  instance_cpu_time_ns{
    app="lion",
    proc="web",
    rev="34d0f99",
    env="prod",
    job="cluster-manager",
    host="localhost",
  }`,
	},
	{
		expr: `instance_cpu_time_ns{app="lion", proc="web", rev="34d0f99", env="prod", job="cluster-manager", host="localhost",}`,
		expected: `  instance_cpu_time_ns{
    app="lion",
    proc="web",
    rev="34d0f99",
    env="prod",
    job="cluster-manager",
    host="localhost",
  }`,
	},
	{
		expr:     `metric_one + metric_two`,
		expected: `    metric_one + metric_two`,
	},
	{
		expr:     `metric_one{foo="bar"} + metric_two{foo="bar"}`,
		expected: `    metric_one{foo="bar"} + metric_two{foo="bar"}`,
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
}

func TestPrettierCases(t *testing.T) {
	for _, expr := range prettierCases {
		p, err := New(PrettifyExpression, expr.expr)
		testutil.Ok(t, err)
		standardizeExprStr := p.expressionFromItems(p.lexItems(expr.expr))
		lexItems := p.sortItems(p.lexItems(expr.expr), true)
		err = p.parseExpr(standardizeExprStr)
		testutil.Ok(t, err)
		output, err := p.prettify(lexItems, 0, "")
		testutil.Ok(t, err)
		testutil.Equals(t, expr.expected, output, "formatting does not match")
	}
}
