import ASTNode, {
  binaryOperatorType,
  nodeType,
  valueType,
  Call,
  compOperatorTypes,
  setOperatorTypes,
} from "./ast";
import { functionArgNames } from "./functionMeta";

export const getNonParenNodeType = (n: ASTNode) => {
  let cur: ASTNode;
  for (cur = n; cur.type === "parenExpr"; cur = cur.expr) {
    // Continue traversing until a non-parenthesis expression is found
  }
  return cur.type;
};

export const isComparisonOperator = (op: binaryOperatorType) => {
  return compOperatorTypes.includes(op);
};

export const isSetOperator = (op: binaryOperatorType) => {
  return setOperatorTypes.includes(op);
};

const binOpPrecedence = {
  [binaryOperatorType.add]: 3,
  [binaryOperatorType.sub]: 3,
  [binaryOperatorType.mul]: 2,
  [binaryOperatorType.div]: 2,
  [binaryOperatorType.mod]: 2,
  [binaryOperatorType.pow]: 1,
  [binaryOperatorType.eql]: 4,
  [binaryOperatorType.neq]: 4,
  [binaryOperatorType.gtr]: 4,
  [binaryOperatorType.lss]: 4,
  [binaryOperatorType.gte]: 4,
  [binaryOperatorType.lte]: 4,
  [binaryOperatorType.and]: 5,
  [binaryOperatorType.or]: 6,
  [binaryOperatorType.unless]: 5,
  [binaryOperatorType.atan2]: 2,
};

export const maybeParenthesizeBinopChild = (
  op: binaryOperatorType,
  child: ASTNode
): ASTNode => {
  if (child.type !== nodeType.binaryExpr) {
    return child;
  }

  if (binOpPrecedence[op] > binOpPrecedence[child.op]) {
    return child;
  }

  // TODO: Parens aren't necessary for left-associativity within same precedence,
  // or right-associativity between two power operators.
  return {
    type: nodeType.parenExpr,
    expr: child,
  };
};

export const getNodeChildren = (node: ASTNode): ASTNode[] => {
  switch (node.type) {
    case nodeType.aggregation:
      return node.param === null ? [node.expr] : [node.param, node.expr];
    case nodeType.subquery:
      return [node.expr];
    case nodeType.parenExpr:
      return [node.expr];
    case nodeType.call:
      return node.args;
    case nodeType.matrixSelector:
    case nodeType.vectorSelector:
    case nodeType.numberLiteral:
    case nodeType.stringLiteral:
      return [];
    case nodeType.placeholder:
      return node.children;
    case nodeType.unaryExpr:
      return [node.expr];
    case nodeType.binaryExpr:
      return [node.lhs, node.rhs];
    default:
      throw new Error("unsupported node type");
  }
};

export const getNodeChild = (node: ASTNode, idx: number) => {
  switch (node.type) {
    case nodeType.aggregation:
      return node.param === null || idx === 1 ? node.expr : node.param;
    case nodeType.subquery:
      return node.expr;
    case nodeType.parenExpr:
      return node.expr;
    case nodeType.call:
      return node.args[idx];
    case nodeType.unaryExpr:
      return node.expr;
    case nodeType.binaryExpr:
      return idx === 0 ? node.lhs : node.rhs;
    default:
      throw new Error("unsupported node type");
  }
};

export const containsPlaceholders = (node: ASTNode): boolean =>
  node.type === nodeType.placeholder ||
  getNodeChildren(node).some((n) => containsPlaceholders(n));

export const nodeValueType = (node: ASTNode): valueType | null => {
  switch (node.type) {
    case nodeType.aggregation:
      return valueType.vector;
    case nodeType.binaryExpr: {
      const childTypes = [nodeValueType(node.lhs), nodeValueType(node.rhs)];

      if (childTypes.includes(null)) {
        // One of the children is or a has a placeholder and thus an undefined type.
        return null;
      }

      if (childTypes.includes(valueType.vector)) {
        return valueType.vector;
      }

      return valueType.scalar;
    }
    case nodeType.call:
      return node.func.returnType;
    case nodeType.matrixSelector:
      return valueType.matrix;
    case nodeType.numberLiteral:
      return valueType.scalar;
    case nodeType.parenExpr:
      return nodeValueType(node.expr);
    case nodeType.placeholder:
      return null;
    case nodeType.stringLiteral:
      return valueType.string;
    case nodeType.subquery:
      return valueType.matrix;
    case nodeType.unaryExpr:
      return nodeValueType(node.expr);
    case nodeType.vectorSelector:
      return valueType.vector;
    default:
      throw new Error("invalid node type");
  }
};

export const childDescription = (node: ASTNode, idx: number): string => {
  switch (node.type) {
    case nodeType.aggregation:
      if (aggregatorsWithParam.includes(node.op) && idx === 0) {
        switch (node.op) {
          case "topk":
          case "bottomk":
          case "limitk":
            return "k";
          case "quantile":
            return "quantile";
          case "count_values":
            return "target label name";
          case "limit_ratio":
            return "ratio";
        }
      }

      return "vector to aggregate";
    case nodeType.binaryExpr:
      return idx === 0 ? "left-hand side" : "right-hand side";
    case nodeType.call:
      if (node.func.name in functionArgNames) {
        const argNames = functionArgNames[node.func.name];
        return argNames[Math.min(argNames.length - 1, idx)];
      }
      return "argument";
    case nodeType.parenExpr:
      return "expression";
    case nodeType.placeholder:
      return "argument";
    case nodeType.subquery:
      return "subquery to execute";
    case nodeType.unaryExpr:
      return "expression";
    default:
      throw new Error("invalid node type");
  }
};

export const aggregatorsWithParam = [
  "topk",
  "bottomk",
  "quantile",
  "count_values",
  "limitk",
  "limit_ratio",
];

export const anyValueType = [
  valueType.scalar,
  valueType.string,
  valueType.matrix,
  valueType.vector,
];

export const allowedChildValueTypes = (
  node: ASTNode,
  idx: number
): valueType[] => {
  switch (node.type) {
    case nodeType.aggregation:
      if (aggregatorsWithParam.includes(node.op) && idx === 0) {
        if (node.op === "count_values") {
          return [valueType.string];
        }
        return [valueType.scalar];
      }

      return [valueType.vector];
    case nodeType.binaryExpr:
      // TODO: Do deeper constraint checking here.
      // - Set ops only between vectors.
      // - Bools only for filter ops.
      // - Advanced: check cardinality.
      return [valueType.scalar, valueType.vector];
    case nodeType.call:
      return [node.func.argTypes[Math.min(idx, node.func.argTypes.length - 1)]];
    case nodeType.parenExpr:
      return anyValueType;
    case nodeType.placeholder:
      return anyValueType;
    case nodeType.subquery:
      return [valueType.vector];
    case nodeType.unaryExpr:
      return anyValueType;
    default:
      throw new Error("invalid node type");
  }
};

export const canAddVarArg = (node: Call): boolean => {
  if (node.func.variadic === -1) {
    return true;
  }

  // TODO: Only works for 1 vararg, but PromQL only has functions with either 1 (not 2, 3, ...) or unlimited (-1) varargs in practice, so this is fine for now.
  return node.args.length < node.func.argTypes.length;
};

export const canRemoveVarArg = (node: Call): boolean => {
  return (
    node.func.variadic !== 0 && node.args.length >= node.func.argTypes.length
  );
};

export const humanizedValueType: Record<valueType, string> = {
  [valueType.none]: "none",
  [valueType.string]: "string",
  [valueType.scalar]: "number (scalar)",
  [valueType.vector]: "instant vector",
  [valueType.matrix]: "range vector",
};

const metricNameRe = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/;
const labelNameCharsetRe = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

export const metricContainsExtendedCharset = (str: string) => {
  return str !== "" && !metricNameRe.test(str);
};

export const labelNameContainsExtendedCharset = (str: string) => {
  return !labelNameCharsetRe.test(str);
};

export const escapeString = (str: string) => {
  return str.replace(/([\\"])/g, "\\$1");
};

export const maybeQuoteLabelName = (str: string) => {
  return labelNameContainsExtendedCharset(str) ? `"${escapeString(str)}"` : str;
};
