package prettier

import (
	"fmt"
	// "reflect"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

type prettierTest struct {
	expr     string
	expected string
}

var exprs = []prettierTest{
	{
		expr: "first + second + third",
		expected: `  first
+
  second
+
  third`,
	},
	{
		expr: `first{foo="bar",a="b", c="d"}`,
		expected: `first{
  a="b",
  c="d",
  foo="bar",
}`,
	},
	{
		expr: `first{c="d",
			foo="bar",a="b",}`,
		expected: `first{
  a="b",
  c="d",
  foo="bar",
}`,
	},
	{
		expr: `first{foo="bar",a="b", c="d"} + second{foo="bar", c="d"}`,
		expected: `  first{
    a="b",
    c="d",
    foo="bar",
  }
+
  second{
    c="d",
    foo="bar",
  }`,
	},
	{
		expr: `(first)`,
		expected: `(
  first
)`,
	},

	{
		expr: `((((first))))`,
		expected: `(
  (
    (
      (
        first
      )
    )
  )
)`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + second{foo="bar", c="d"}))))`,
		expected: `(
  (
    (
      (
          first{
            a="b",
            c="d",
            foo="bar",
          }
        +
          second{
            c="d",
            foo="bar",
          }
      )
    )
  )
)`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + ((second{foo="bar", c="d"}))))))`,
		expected: `(
  (
    (
      (
          first{
            a="b",
            c="d",
            foo="bar",
          }
        +
          (
            (
              second{
                c="d",
                foo="bar",
              }
            )
          )
      )
    )
  )
)`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + ((second{foo="bar", c="d"})) + third{foo="bar",c="d"}))))`,
		expected: `(
  (
    (
      (
          first{
            a="b",
            c="d",
            foo="bar",
          }
        +
          (
            (
              second{
                c="d",
                foo="bar",
              }
            )
          )
        +
          third{
            c="d",
            foo="bar",
          }
      )
    )
  )
)`,
	},
	{
		expr: `
    # head 1
    # head 2
    first # comment 1
    # comment 2
    > bool second`,
		expected: `  # head 1
  # head 2
  first # comment 1
  # comment 2
> bool
  second
`,
	},
	{
		expr: `# head 1
    # head 2
    first{foo="bar", a="b"} # comment 1
    # comment 2
    > bool second{foo="bar", c="d"}
`, expected: `  # head 1
  # head 2
  first{
    a="b",
    foo="bar",
  } # comment 1
  # comment 2
> bool
  second{
    c="d",
    foo="bar",
  }
`,
	},
}

// func TestPrettify(t *testing.T) {
// 	for _, expr := range exprs {
// 		p, err := New(PrettifyExpression, expr.expr)
// 		testutil.Ok(t, err)
// 		// err := p.parseExpr(expr.expr)
// 		// testutil.Ok(t, err)
// 		// formatted, err := p.Prettify(expression, reflect.TypeOf(""), 0, "")
// 		// testutil.Ok(t, err)
// 		// testutil.Equals(t, expr.expected, formatted, "formatting does not match")
// 	}
// }

var exprsItems = []prettierTest{
	{
		expr: "first + second + third",
		expected: `  first
+
  second
+
  third
`,
	},
	{
		expr: `first{foo="bar",a="b", c="d"}`,
		expected: `  first{
    foo="bar",
    a="b",
    c="d",
  }
`,
	},
	{
		expr: `first{c="d",
			foo="bar",a="b",}`,
		expected: `  first{
    c="d",
    foo="bar",
    a="b",
  }
`,
	},
	{
		expr: `first{foo="bar",a="b", c="d"} + second{foo="bar", c="d"}`,
		expected: `  first{
    foo="bar",
    a="b",
    c="d",
  }
+
  second{
    foo="bar",
    c="d",
  }
`,
	},
	{
		expr: `(first)`,
		expected: `  (
    first
  )
`,
	},

	{
		expr: `((((first))))`,
		expected: `  (
    (
      (
        (
          first
        )
      )
    )
  )
`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + second{foo="bar", c="d"}))))`,
		expected: `  (
    (
      (
        (
          first{
            foo="bar",
            a="b",
            c="d",
          }
        +
          second{
            foo="bar",
            c="d",
          }
        )
      )
    )
  )
`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + ((second{foo="bar", c="d"}))))))`,
		expected: `  (
    (
      (
        (
          first{
            foo="bar",
            a="b",
            c="d",
          }
        +
          (
            (
              second{
                foo="bar",
                c="d",
              }
            )
          )
        )
      )
    )
  )
`,
	},
	{
		expr: `((((first{foo="bar",a="b", c="d"} + ((second{foo="bar", c="d"})) + third{foo="bar",c="d"}))))`,
		expected: `  (
    (
      (
        (
          first{
            foo="bar",
            a="b",
            c="d",
          }
        +
          (
            (
              second{
                foo="bar",
                c="d",
              }
            )
          )
        +
          third{
            foo="bar",
            c="d",
          }
        )
      )
    )
  )
`,
	},
	{
		expr: `
    # head 1
    # head 2
    first # comment 1
    # comment 2
    > bool second`,
		expected: `  # head 1
  # head 2
  first  # comment 1
  # comment 2
> bool
  second
`,
	},
	{
		expr: `# head 1
    # head 2
    first{foo="bar", a="b"} # comment 1
    # comment 2
    > bool second{foo="bar", c="d"}
`, expected: `  # head 1
  # head 2
  first{
    foo="bar",
    a="b",
  }  # comment 1
  # comment 2
> bool
  second{
    foo="bar",
    c="d",
  }
`,
	},
}

func TestPrettifyItems(t *testing.T) {
	for _, expr := range exprsItems {
		p, err := New(PrettifyExpression, expr.expr)
		testutil.Ok(t, err)
		// expression, err := p.parseExpr(expr.expr)
		lexItems := p.lexItems(expr.expr)
		p.pd.buff = 1
		output := p.prettifyItems(lexItems, 0, "")
		fmt.Println(output)
		// testutil.Ok(t, err)
		// formatted, err := p.Prettify(expression, reflect.TypeOf(""), 0, "")
		// testutil.Ok(t, err)
		testutil.Equals(t, expr.expected, output, "formatting does not match")
	}
}

// =======================================================================

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
    first="second",
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
      hello="world",
    }
  +
    second{
      foo="bar",
    }`,
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
		fmt.Println(output)
		testutil.Equals(t, expr.expected, output, "formatting does not match")
	}
}
