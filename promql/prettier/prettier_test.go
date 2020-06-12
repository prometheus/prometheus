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
		expected: `
    first
  +
    second`,
	},
	prettierTest{
		expr: `first{foo="bar", hello="world"} + second{foo="bar"}`,
		expected: `
    first{
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
		expected: `
    first{
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
