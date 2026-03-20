import { describe, expect, it } from "vitest";
import {
  getNonParenNodeType,
  containsPlaceholders,
  nodeValueType,
} from "./utils";
import { nodeType, valueType, binaryOperatorType } from "./ast";

describe("getNonParenNodeType", () => {
  it("works for non-paren type", () => {
    expect(
      getNonParenNodeType({ type: nodeType.numberLiteral, val: "1" })
    ).toBe(nodeType.numberLiteral);
  });

  it("works for single parentheses wrapper", () => {
    expect(
      getNonParenNodeType({
        type: nodeType.parenExpr,
        expr: {
          type: nodeType.numberLiteral,
          val: "1",
        },
      })
    ).toBe(nodeType.numberLiteral);
  });

  it("works for multiple parentheses wrappers", () => {
    expect(
      getNonParenNodeType({
        type: nodeType.parenExpr,
        expr: {
          type: nodeType.parenExpr,
          expr: {
            type: nodeType.parenExpr,
            expr: {
              type: nodeType.numberLiteral,
              val: "1",
            },
          },
        },
      })
    ).toBe(nodeType.numberLiteral);
  });
});

describe("containsPlaceholders", () => {
  it("does not find placeholders in complete expressions", () => {
    expect(
      containsPlaceholders({
        type: nodeType.parenExpr,
        expr: {
          type: nodeType.numberLiteral,
          val: "1",
        },
      })
    ).toBe(false);
  });

  it("finds placeholders at the root", () => {
    expect(
      containsPlaceholders({
        type: nodeType.placeholder,
        children: [],
      })
    ).toBe(true);
  });

  it("finds placeholders in nested expressions with placeholders", () => {
    expect(
      containsPlaceholders({
        type: nodeType.parenExpr,
        expr: {
          type: nodeType.placeholder,
          children: [],
        },
      })
    ).toBe(true);
  });
});

describe("nodeValueType", () => {
  it("works for binary expressions with placeholders", () => {
    expect(
      nodeValueType({
        type: nodeType.binaryExpr,
        op: binaryOperatorType.add,
        lhs: { type: nodeType.placeholder, children: [] },
        rhs: { type: nodeType.placeholder, children: [] },
        matching: null,
        bool: false,
      })
    ).toBeNull();
  });

  it("works for scalar-scalar binops", () => {
    expect(
      nodeValueType({
        type: nodeType.binaryExpr,
        op: binaryOperatorType.add,
        lhs: { type: nodeType.numberLiteral, val: "1" },
        rhs: { type: nodeType.numberLiteral, val: "1" },
        matching: null,
        bool: false,
      })
    ).toBe(valueType.scalar);
  });

  it("works for scalar-vector binops", () => {
    expect(
      nodeValueType({
        type: nodeType.binaryExpr,
        op: binaryOperatorType.add,
        lhs: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        rhs: { type: nodeType.numberLiteral, val: "1" },
        matching: null,
        bool: false,
      })
    ).toBe(valueType.vector);
  });

  it("works for vector-vector binops", () => {
    expect(
      nodeValueType({
        type: nodeType.binaryExpr,
        op: binaryOperatorType.add,
        lhs: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        rhs: {
          type: nodeType.vectorSelector,
          name: "metric_name",
          matchers: [],
          offset: 0,
          timestamp: null,
          startOrEnd: null,
          anchored: false,
          smoothed: false,
        },
        matching: null,
        bool: false,
      })
    ).toBe(valueType.vector);
  });
});
