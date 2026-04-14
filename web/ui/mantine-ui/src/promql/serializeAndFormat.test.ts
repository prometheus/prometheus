import { describe, expect, it } from "vitest";
import serializeNode from "./serialize";
import ASTNode, {
  nodeType,
  matchType,
  aggregationType,
  unaryOperatorType,
  binaryOperatorType,
  vectorMatchCardinality,
} from "./ast";
import { functionSignatures } from "./functionSignatures";
import { formatNode } from "./format";
import { render } from "@testing-library/react";

describe("serializeNode and formatNode", () => {
  it("should serialize correctly", () => {
    const tests: { node: ASTNode; output: string; prettyOutput?: string }[] = [
      // Vector selectors.
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: "metric_name",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [
            { type: matchType.equal, name: "label1", value: "value1" },
            { type: matchType.notEqual, name: "label2", value: "value2" },
            { type: matchType.matchRegexp, name: "label3", value: "value3" },
            { type: matchType.matchNotRegexp, name: "label4", value: "value4" },
          ],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output:
          'metric_name{label1="value1",label2!="value2",label3=~"value3",label4!~"value4"}',
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 60000,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: "metric_name offset 1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: -60000,
          timestamp: null,
          startOrEnd: "start",
          anchored: false,
          smoothed: false,
        },
        output: "metric_name @ start() offset -1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: -60000,
          timestamp: null,
          startOrEnd: "end",
          anchored: false,
          smoothed: false,
        },
        output: "metric_name @ end() offset -1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: -60000,
          timestamp: 123000,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: "metric_name @ 123.000 offset -1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "",
          matchers: [
            { type: matchType.equal, name: "__name__", value: "metric_name" },
          ],
          offset: 60000,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: "metric_name offset 1m",
      },
      {
        // Escaping in label values.
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [{ type: matchType.equal, name: "label1", value: '"""' }],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: 'metric_name{label1="\\"\\"\\""}',
      },

      // Matrix selectors.
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [
            { type: matchType.equal, name: "label1", value: "value1" },
            { type: matchType.notEqual, name: "label2", value: "value2" },
            { type: matchType.matchRegexp, name: "label3", value: "value3" },
            { type: matchType.matchNotRegexp, name: "label4", value: "value4" },
          ],
          range: 300000,
          offset: 600000,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output:
          'metric_name{label1="value1",label2!="value2",label3=~"value3",label4!~"value4"}[5m] offset 10m',
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: -600000,
          timestamp: 123000,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: "metric_name[5m] @ 123.000 offset -10m",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: -600000,
          timestamp: null,
          startOrEnd: "start",
          anchored: false,
          smoothed: false,
        },
        output: "metric_name[5m] @ start() offset -10m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "", // Test formatting a selector with an empty metric name.
          matchers: [
            { type: matchType.equal, name: "label1", value: "value1" },
          ],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        output: '{label1="value1"}',
      },

      // Anchored and smoothed modifiers.
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name anchored",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: true,
        },
        output: "metric_name smoothed",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 60000,
          timestamp: null,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name anchored offset 1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 60000,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: true,
        },
        output: "metric_name smoothed offset 1m",
      },
      {
        node: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: 123000,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name anchored @ 123.000",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name[5m] anchored",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: true,
        },
        output: "metric_name[5m] smoothed",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 600000,
          timestamp: null,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name[5m] anchored offset 10m",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 600000,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: true,
        },
        output: "metric_name[5m] smoothed offset 10m",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 0,
          timestamp: 123000,
          startOrEnd: null,
          anchored: true,
          smoothed: false,
        },
        output: "metric_name[5m] anchored @ 123.000",
      },
      {
        node: {
          type: nodeType.matrixSelector,
          name: "metric_name",
          matchers: [],
          range: 300000,
          offset: 600000,
          timestamp: 123000,
          startOrEnd: null,
          anchored: false,
          smoothed: true,
        },
        output: "metric_name[5m] smoothed @ 123.000 offset 10m",
      },

      // Aggregations.
      {
        node: {
          type: nodeType.aggregation,
          expr: { type: nodeType.placeholder, children: [] },
          op: aggregationType.sum,
          param: null,
          grouping: [],
          without: false,
        },
        output: "sum(…)",
        prettyOutput: `sum(
  …
)`,
      },
      {
        node: {
          type: nodeType.aggregation,
          expr: { type: nodeType.placeholder, children: [] },
          op: aggregationType.topk,
          param: { type: nodeType.numberLiteral, val: "3" },
          grouping: [],
          without: false,
        },
        output: "topk(3, …)",
        prettyOutput: `topk(
  3,
  …
)`,
      },
      {
        node: {
          type: nodeType.aggregation,
          expr: { type: nodeType.placeholder, children: [] },
          op: aggregationType.sum,
          param: null,
          grouping: [],
          without: true,
        },
        output: "sum without() (…)",
        prettyOutput: `sum without() (
  …
)`,
      },
      {
        node: {
          type: nodeType.aggregation,
          expr: { type: nodeType.placeholder, children: [] },
          op: aggregationType.sum,
          param: null,
          grouping: ["label1", "label2"],
          without: false,
        },
        output: "sum by(label1, label2) (…)",
        prettyOutput: `sum by(label1, label2) (
  …
)`,
      },
      {
        node: {
          type: nodeType.aggregation,
          expr: { type: nodeType.placeholder, children: [] },
          op: aggregationType.sum,
          param: null,
          grouping: ["label1", "label2"],
          without: true,
        },
        output: "sum without(label1, label2) (…)",
        prettyOutput: `sum without(label1, label2) (
  …
)`,
      },

      // Subqueries.
      {
        node: {
          type: nodeType.subquery,
          expr: { type: nodeType.placeholder, children: [] },
          range: 300000,
          offset: 0,
          step: 0,
          timestamp: null,
          startOrEnd: null,
        },
        output: "…[5m:]",
      },
      {
        node: {
          type: nodeType.subquery,
          expr: { type: nodeType.placeholder, children: [] },
          range: 300000,
          offset: 600000,
          step: 60000,
          timestamp: null,
          startOrEnd: null,
        },
        output: "…[5m:1m] offset 10m",
      },
      {
        node: {
          type: nodeType.subquery,
          expr: { type: nodeType.placeholder, children: [] },
          range: 300000,
          offset: -600000,
          step: 60000,
          timestamp: 123000,
          startOrEnd: null,
        },
        output: "…[5m:1m] @ 123.000 offset -10m",
      },
      {
        node: {
          type: nodeType.subquery,
          expr: { type: nodeType.placeholder, children: [] },
          range: 300000,
          offset: -600000,
          step: 60000,
          timestamp: null,
          startOrEnd: "end",
        },
        output: "…[5m:1m] @ end() offset -10m",
      },
      {
        node: {
          type: nodeType.subquery,
          expr: {
            type: nodeType.call,
            func: functionSignatures["rate"],
            args: [
              {
                type: nodeType.matrixSelector,
                range: 600000,
                name: "metric_name",
                matchers: [],
                offset: 0,
                timestamp: null,
                startOrEnd: null,
                anchored: false,
                smoothed: false,
              },
            ],
          },
          range: 300000,
          offset: 0,
          step: 0,
          timestamp: null,
          startOrEnd: null,
        },
        output: "rate(metric_name[10m])[5m:]",
        prettyOutput: `rate(
  metric_name[10m]
)[5m:]`,
      },

      // Parentheses.
      {
        node: {
          type: nodeType.parenExpr,
          expr: { type: nodeType.placeholder, children: [] },
        },
        output: "(…)",
        prettyOutput: `(
  …
)`,
      },

      // Call.
      {
        node: {
          type: nodeType.call,
          func: functionSignatures["time"],
          args: [],
        },
        output: "time()",
      },
      {
        node: {
          type: nodeType.call,
          func: functionSignatures["rate"],
          args: [{ type: nodeType.placeholder, children: [] }],
        },
        output: "rate(…)",
        prettyOutput: `rate(
  …
)`,
      },
      {
        node: {
          type: nodeType.call,
          func: functionSignatures["label_join"],
          args: [
            { type: nodeType.placeholder, children: [] },
            { type: nodeType.stringLiteral, val: "foo" },
            { type: nodeType.stringLiteral, val: "bar" },
            { type: nodeType.stringLiteral, val: "baz" },
          ],
        },
        output: 'label_join(…, "foo", "bar", "baz")',
        prettyOutput: `label_join(
  …,
  "foo",
  "bar",
  "baz"
)`,
      },

      // Number literals.
      {
        node: {
          type: nodeType.numberLiteral,
          val: "1.2345",
        },
        output: "1.2345",
      },

      // String literals.
      {
        node: {
          type: nodeType.stringLiteral,
          val: 'hello, " world',
        },
        output: '"hello, \\" world"',
      },

      // Unary expressions.
      {
        node: {
          type: nodeType.unaryExpr,
          expr: { type: nodeType.placeholder, children: [] },
          op: unaryOperatorType.minus,
        },
        output: "-…",
        prettyOutput: "-…",
      },
      {
        node: {
          type: nodeType.unaryExpr,
          expr: { type: nodeType.placeholder, children: [] },
          op: unaryOperatorType.plus,
        },
        output: "+…",
        prettyOutput: "+…",
      },
      {
        node: {
          type: nodeType.unaryExpr,
          expr: {
            type: nodeType.parenExpr,
            expr: { type: nodeType.placeholder, children: [] },
          },
          op: unaryOperatorType.minus,
        },
        output: "-(…)",
        prettyOutput: `-(
  …
)`,
      },
      {
        // Nested indentation.
        node: {
          type: nodeType.unaryExpr,
          expr: {
            type: nodeType.aggregation,
            op: aggregationType.sum,
            expr: {
              type: nodeType.unaryExpr,
              expr: {
                type: nodeType.parenExpr,
                expr: { type: nodeType.placeholder, children: [] },
              },
              op: unaryOperatorType.minus,
            },
            grouping: [],
            param: null,
            without: false,
          },
          op: unaryOperatorType.minus,
        },
        output: "-sum(-(…))",
        prettyOutput: `-sum(
  -(
    …
  )
)`,
      },

      // Binary expressions.
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: null,
          bool: false,
        },
        output: "… + …",
        prettyOutput: `  …
+
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            labels: [],
            on: false,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + …",
        prettyOutput: `  …
+
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            labels: [],
            on: true,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on() …",
        prettyOutput: `  …
+ on()
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            labels: ["label1", "label2"],
            on: true,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on(label1, label2) …",
        prettyOutput: `  …
+ on(label1, label2)
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            labels: ["label1", "label2"],
            on: false,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + ignoring(label1, label2) …",
        prettyOutput: `  …
+ ignoring(label1, label2)
  …`,
      },
      {
        // Empty ignoring() without group modifiers can be stripped away.
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            labels: [],
            on: false,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + …",
        prettyOutput: `  …
+
  …`,
      },
      {
        // Empty ignoring() with group modifiers may not be stripped away.
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.manyToOne,
            labels: [],
            on: false,
            include: ["__name__"],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + ignoring() group_left(__name__) …",
        prettyOutput: `  …
+ ignoring() group_left(__name__)
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToMany,
            labels: ["label1", "label2"],
            on: true,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on(label1, label2) group_right() …",
        prettyOutput: `  …
+ on(label1, label2) group_right()
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToMany,
            labels: ["label1", "label2"],
            on: true,
            include: ["label3"],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on(label1, label2) group_right(label3) …",
        prettyOutput: `  …
+ on(label1, label2) group_right(label3)
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.manyToOne,
            labels: ["label1", "label2"],
            on: true,
            include: [],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on(label1, label2) group_left() …",
        prettyOutput: `  …
+ on(label1, label2) group_left()
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.add,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.manyToOne,
            labels: ["label1", "label2"],
            on: true,
            include: ["label3"],
            fillValues: { lhs: null, rhs: null },
          },
          bool: false,
        },
        output: "… + on(label1, label2) group_left(label3) …",
        prettyOutput: `  …
+ on(label1, label2) group_left(label3)
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.eql,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: null,
          bool: true,
        },
        output: "… == bool …",
        prettyOutput: `  …
== bool
  …`,
      },
      {
        node: {
          type: nodeType.binaryExpr,
          op: binaryOperatorType.eql,
          lhs: { type: nodeType.placeholder, children: [] },
          rhs: { type: nodeType.placeholder, children: [] },
          matching: {
            card: vectorMatchCardinality.oneToMany,
            labels: ["label1", "label2"],
            on: true,
            include: ["label3"],
            fillValues: { lhs: null, rhs: null },
          },
          bool: true,
        },
        output: "… == bool on(label1, label2) group_right(label3) …",
        prettyOutput: `  …
== bool on(label1, label2) group_right(label3)
  …`,
      },
      // Test new Prometheus 3.0 UTF-8 support.
      {
        node: {
          bool: false,
          lhs: {
            bool: false,
            lhs: {
              expr: {
                matchers: [
                  {
                    name: "__name__",
                    type: matchType.equal,
                    value: "metric_ä",
                  },
                  {
                    name: "foo",
                    type: matchType.equal,
                    value: "bar",
                  },
                ],
                name: "",
                offset: 0,
                startOrEnd: null,
                timestamp: null,
                type: nodeType.vectorSelector,
                anchored: false,
                smoothed: false,
              },
              grouping: ["a", "ä"],
              op: aggregationType.sum,
              param: null,
              type: nodeType.aggregation,
              without: false,
            },
            matching: {
              card: vectorMatchCardinality.manyToOne,
              include: ["c", "ü"],
              labels: ["b", "ö"],
              on: true,
              fillValues: { lhs: null, rhs: null },
            },
            op: binaryOperatorType.div,
            rhs: {
              expr: {
                matchers: [
                  {
                    name: "__name__",
                    type: matchType.equal,
                    value: "metric_ö",
                  },
                  {
                    name: "bar",
                    type: matchType.equal,
                    value: "foo",
                  },
                ],
                name: "",
                offset: 0,
                startOrEnd: null,
                timestamp: null,
                type: nodeType.vectorSelector,
                anchored: false,
                smoothed: false,
              },
              grouping: ["d", "ä"],
              op: aggregationType.sum,
              param: null,
              type: nodeType.aggregation,
              without: true,
            },
            type: nodeType.binaryExpr,
          },
          matching: {
            card: vectorMatchCardinality.oneToOne,
            include: [],
            labels: ["e", "ö"],
            on: false,
            fillValues: { lhs: null, rhs: null },
          },
          op: binaryOperatorType.add,
          rhs: {
            expr: {
              matchers: [
                {
                  name: "__name__",
                  type: matchType.equal,
                  value: "metric_ü",
                },
              ],
              name: "",
              offset: 0,
              startOrEnd: null,
              timestamp: null,
              type: nodeType.vectorSelector,
              anchored: false,
              smoothed: false,
            },
            type: nodeType.parenExpr,
          },
          type: nodeType.binaryExpr,
        },
        output:
          'sum by(a, "ä") ({"metric_ä",foo="bar"}) / on(b, "ö") group_left(c, "ü") sum without(d, "ä") ({"metric_ö",bar="foo"}) + ignoring(e, "ö") ({"metric_ü"})',
        prettyOutput: `    sum by(a, "ä") (
      {"metric_ä",foo="bar"}
    )
  / on(b, "ö") group_left(c, "ü")
    sum without(d, "ä") (
      {"metric_ö",bar="foo"}
    )
+ ignoring(e, "ö")
  (
    {"metric_ü"}
  )`,
      },
    ];

    tests.forEach((t) => {
      expect(serializeNode(t.node)).toBe(t.output);
      expect(serializeNode(t.node, 0, true)).toBe(
        t.prettyOutput !== undefined ? t.prettyOutput : t.output
      );

      const { container } = render(formatNode(t.node, true));
      expect(container.textContent).toBe(t.output);
    });
  });
});
