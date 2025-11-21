export enum nodeType {
  aggregation = "aggregation",
  binaryExpr = "binaryExpr",
  call = "call",
  matrixSelector = "matrixSelector",
  subquery = "subquery",
  numberLiteral = "numberLiteral",
  parenExpr = "parenExpr",
  stringLiteral = "stringLiteral",
  unaryExpr = "unaryExpr",
  vectorSelector = "vectorSelector",
  placeholder = "placeholder",
}

export enum aggregationType {
  sum = "sum",
  min = "min",
  max = "max",
  avg = "avg",
  stddev = "stddev",
  stdvar = "stdvar",
  count = "count",
  group = "group",
  countValues = "count_values",
  bottomk = "bottomk",
  topk = "topk",
  quantile = "quantile",
  limitK = "limitk",
  limitRatio = "limit_ratio",
}

export enum binaryOperatorType {
  add = "+",
  sub = "-",
  mul = "*",
  div = "/",
  mod = "%",
  pow = "^",
  eql = "==",
  neq = "!=",
  gtr = ">",
  lss = "<",
  gte = ">=",
  lte = "<=",
  and = "and",
  or = "or",
  unless = "unless",
  atan2 = "atan2",
}

export const compOperatorTypes: binaryOperatorType[] = [
  binaryOperatorType.eql,
  binaryOperatorType.neq,
  binaryOperatorType.gtr,
  binaryOperatorType.lss,
  binaryOperatorType.gte,
  binaryOperatorType.lte,
];

export const setOperatorTypes: binaryOperatorType[] = [
  binaryOperatorType.and,
  binaryOperatorType.or,
  binaryOperatorType.unless,
];

export enum unaryOperatorType {
  plus = "+",
  minus = "-",
}

export enum vectorMatchCardinality {
  oneToOne = "one-to-one",
  manyToOne = "many-to-one",
  oneToMany = "one-to-many",
  manyToMany = "many-to-many",
}

export enum valueType {
  // TODO: 'none' should never make it out of Prometheus. Do we need this here?
  none = "none",
  vector = "vector",
  scalar = "scalar",
  matrix = "matrix",
  string = "string",
}

export enum matchType {
  equal = "=",
  notEqual = "!=",
  matchRegexp = "=~",
  matchNotRegexp = "!~",
}

export interface Func {
  name: string;
  argTypes: valueType[];
  variadic: number;
  returnType: valueType;
}

export interface LabelMatcher {
  type: matchType;
  name: string;
  value: string;
}

export interface VectorMatching {
  card: vectorMatchCardinality;
  labels: string[];
  on: boolean;
  include: string[];
}

export type StartOrEnd = "start" | "end" | null;

// AST Node Types.

export interface Aggregation {
  type: nodeType.aggregation;
  expr: ASTNode;
  op: aggregationType;
  param: ASTNode | null;
  grouping: string[];
  without: boolean;
}

export interface BinaryExpr {
  type: nodeType.binaryExpr;
  op: binaryOperatorType;
  lhs: ASTNode;
  rhs: ASTNode;
  matching: VectorMatching | null;
  bool: boolean;
}

export interface Call {
  type: nodeType.call;
  func: Func;
  args: ASTNode[];
}

export interface MatrixSelector {
  type: nodeType.matrixSelector;
  name: string;
  matchers: LabelMatcher[];
  range: number;
  offset: number;
  timestamp: number | null;
  startOrEnd: StartOrEnd;
  anchored: boolean;
  smoothed: boolean;
}

export interface Subquery {
  type: nodeType.subquery;
  expr: ASTNode;
  range: number;
  offset: number;
  step: number;
  timestamp: number | null;
  startOrEnd: StartOrEnd;
}

export interface NumberLiteral {
  type: nodeType.numberLiteral;
  val: string; // Can't be 'number' because JS doesn't support NaN/Inf/-Inf etc.
}

export interface ParenExpr {
  type: nodeType.parenExpr;
  expr: ASTNode;
}

export interface StringLiteral {
  type: nodeType.stringLiteral;
  val: string;
}

export interface UnaryExpr {
  type: nodeType.unaryExpr;
  op: unaryOperatorType;
  expr: ASTNode;
}

export interface VectorSelector {
  type: nodeType.vectorSelector;
  name: string;
  matchers: LabelMatcher[];
  offset: number;
  timestamp: number | null;
  startOrEnd: StartOrEnd;
  anchored: boolean;
  smoothed: boolean;
}

export interface Placeholder {
  type: nodeType.placeholder;
  children: ASTNode[];
}

type ASTNode =
  | Aggregation
  | BinaryExpr
  | Call
  | MatrixSelector
  | Subquery
  | NumberLiteral
  | ParenExpr
  | StringLiteral
  | UnaryExpr
  | VectorSelector
  | Placeholder;

export default ASTNode;
