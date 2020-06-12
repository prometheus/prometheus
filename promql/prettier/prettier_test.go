package prettier

import (
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

type prettierTest struct {
	expr     string
	expected string
}

var combineCases = []prettierTest{
	prettierTest{
		expr: `(metric_name)`,
		expected: `  (
    metric_name
  )`,
	},
	prettierTest{
		expr: `((((metric_name))))`,
		expected: `  (
    (
      (
        (
          metric_name
        )
      )
    )
  )`,
	},
	prettierTest{
		expr: `(metric_name)`,
		expected: `  (
    metric_name
  )`,
	},
	prettierTest{
		expr: `metric_name{foo="bar", a="b", c="d",first="second"}`,
		expected: `  metric_name{
    foo="bar",
    a="b",
    c="d",
    first="second"
  }`,
	},
	prettierTest{
		expr: `first + second`,
		expected: `    first
  +
    second`,
	},
	prettierTest{
		expr: `first{foo="bar", hello="world"} + second{foo="bar"}`,
		expected: `    first{
      foo="bar",
      hello="world"
    }
  +
    second{
      foo="bar"
    }`,
	},
	prettierTest{
		expr: `first{foo="bar", hello="world"} + second{foo="bar"} + third{foo="bar", localhost="9090"} + forth`,
		expected: `    first{
      foo="bar",
      hello="world"
    }
  +
    second{
      foo="bar"
    }
  +
    third{
      foo="bar",
      localhost="9090"
    }
  +
    forth`,
	},
	prettierTest{
		expr: `first{ # comment
      foo="bar",hello="world" # comment
      }`,
		expected: `  first{
  # comment
    foo="bar",
    hello="world"  # comment
  }`,
	},
	prettierTest{
		expr: `(first{foo="bar", hello="world"} + ((second{foo="bar"})) + (third{foo="bar", localhost="9090"}) + forth)`,
		expected: `  (
      first{
        foo="bar",
        hello="world"
      }
    +
      (
        (
          second{
            foo="bar"
          }
        )
      )
    +
      (
        third{
        foo="bar",
        localhost="9090"
        }
      )
    +
      forth
  )`,
	},
	prettierTest{
		expr: `(first{foo="bar", hello="world"} + ((second{foo="bar", gfg="ghg"})) + ((third{foo="bar", localhost="9090"})) + (forth))`,
		expected: `  (
      first{
        foo="bar",
        hello="world"
      }
    +
      (
        (
          second{
            foo="bar",
            gfg="ghg"
          }
        )
      )
    +
      (
        (
          third{
          foo="bar",
          localhost="9090"
          }
        )
      )
    +
      (
        forth
      )
  )`,
	},
	// Aggregate Expressions
	prettierTest{
		expr: `sum(metric_name)`,
		expected: `  sum(
    metric_name
  )`,
	},
	prettierTest{
		expr: `count(metric_name)`,
		expected: `  count(
    metric_name
  )`,
	},
	prettierTest{
		expr: `stddev(metric_name)`,
		expected: `  stddev(
    metric_name
  )`,
	},
	prettierTest{
		expr: `quantile(0.95, metric_name)`,
		expected: `  quantile(0.95,
    metric_name
  )`,
	},
	prettierTest{
		expr: `topk(5, metric_name)`,
		expected: `  topk(5,
    metric_name
  )`,
	},
	prettierTest{
		expr: `sum without(label) (metric_name)`,
		expected: `  sum without (label) (
    metric_name
  )`,
	},
	prettierTest{
		expr: `sum by(label) (metric_name)`,
		expected: `  sum by (label) (
    metric_name
  )`,
	},
	prettierTest{
		expr: `sum (metric_name) without(label)`,
		expected: `  sum(
    metric_name
  )   without (label) `,
	},
	prettierTest{
		expr: `sum (metric_name) by(label)`,
		expected: `  sum(
    metric_name
  )   by (label) `,
	},
	prettierTest{
		expr: `topk without (label) (5, metric_name)`,
		expected: `  topk without (label) (5,
    metric_name
  )`,
	},
}

func TestCombinePrettify(t *testing.T) {
	for _, expr := range combineCases {
		p, err := New(PrettifyExpression, expr.expr)
		testutil.Ok(t, err)
		lexItems := p.lexItems(expr.expr)
		p.parseExpr(expr.expr)
		p.pd.buff = 1
		output, err := p.prettify(lexItems, 0, "")
		testutil.Ok(t, err)
		testutil.Equals(t, expr.expected, output, "formatting does not match")
	}
}
