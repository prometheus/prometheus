import { formatPrometheusDuration } from "../lib/formatTime";
import ASTNode, {
  VectorSelector,
  matchType,
  vectorMatchCardinality,
  nodeType,
  StartOrEnd,
  MatrixSelector,
} from "./ast";
import {
  aggregatorsWithParam,
  maybeParenthesizeBinopChild,
  escapeString,
  metricContainsExtendedCharset,
  maybeQuoteLabelName,
} from "./utils";

const labelNameList = (labels: string[]): string => {
  return labels.map((ln) => maybeQuoteLabelName(ln)).join(", ");
};

const serializeAtAndOffset = (
  timestamp: number | null,
  startOrEnd: StartOrEnd,
  offset: number
): string =>
  `${timestamp !== null ? ` @ ${(timestamp / 1000).toFixed(3)}` : startOrEnd !== null ? ` @ ${startOrEnd}()` : ""}${
    offset === 0
      ? ""
      : offset > 0
        ? ` offset ${formatPrometheusDuration(offset)}`
        : ` offset -${formatPrometheusDuration(-offset)}`
  }`;

const serializeSelector = (node: VectorSelector | MatrixSelector): string => {
  const matchers = node.matchers
    .filter((m) => !(m.name === "__name__" && m.type === matchType.equal))
    .map(
      (m) => `${maybeQuoteLabelName(m.name)}${m.type}"${escapeString(m.value)}"`
    );

  // If the metric name contains the new extended charset, we need to escape it
  // and add it at the beginning of the matchers list in the curly braces.
  const metricName =
    node.name ||
    node.matchers.find(
      (m) => m.name === "__name__" && m.type === matchType.equal
    )?.value ||
    "";
  const metricExtendedCharset = metricContainsExtendedCharset(metricName);
  if (metricExtendedCharset) {
    matchers.unshift(`"${escapeString(metricName)}"`);
  }

  const range =
    node.type === nodeType.matrixSelector
      ? `[${formatPrometheusDuration(node.range)}]`
      : "";
  const extendedAttribute = node.anchored
    ? " anchored"
    : node.smoothed
      ? " smoothed"
      : "";
  const atAndOffset = serializeAtAndOffset(
    node.timestamp,
    node.startOrEnd,
    node.offset
  );

  return `${!metricExtendedCharset ? metricName : ""}${matchers.length > 0 ? `{${matchers.join(",")}}` : ""}${range}${extendedAttribute}${atAndOffset}`;
};

const serializeNode = (
  node: ASTNode,
  indent = 0,
  pretty = false,
  initialIndent = true
): string => {
  const childListSeparator = pretty ? "\n" : "";
  const childSeparator = pretty ? "\n" : " ";
  const childIndent = indent + 2;
  const ind = pretty ? " ".repeat(indent) : "";
  // Needed for unary operators.
  const initialInd = initialIndent ? ind : "";

  switch (node.type) {
    case nodeType.aggregation:
      return `${initialInd}${node.op}${
        node.without
          ? ` without(${labelNameList(node.grouping)}) `
          : node.grouping.length > 0
            ? ` by(${labelNameList(node.grouping)}) `
            : ""
      }(${childListSeparator}${
        aggregatorsWithParam.includes(node.op) && node.param !== null
          ? `${serializeNode(node.param, childIndent, pretty)},${childSeparator}`
          : ""
      }${serializeNode(node.expr, childIndent, pretty)}${childListSeparator}${ind})`;

    case nodeType.subquery:
      return `${initialInd}${serializeNode(node.expr, indent, pretty)}[${formatPrometheusDuration(node.range)}:${
        node.step !== 0 ? formatPrometheusDuration(node.step) : ""
      }]${serializeAtAndOffset(node.timestamp, node.startOrEnd, node.offset)}`;

    case nodeType.parenExpr:
      return `${initialInd}(${childListSeparator}${serializeNode(
        node.expr,
        childIndent,
        pretty
      )}${childListSeparator}${ind})`;

    case nodeType.call: {
      const sep = node.args.length > 0 ? childListSeparator : "";

      return `${initialInd}${node.func.name}(${sep}${node.args
        .map((arg) => serializeNode(arg, childIndent, pretty))
        .join("," + childSeparator)}${sep}${node.args.length > 0 ? ind : ""})`;
    }

    case nodeType.matrixSelector:
      return `${initialInd}${serializeSelector(node)}`;

    case nodeType.vectorSelector:
      return `${initialInd}${serializeSelector(node)}`;

    case nodeType.numberLiteral:
      return `${initialInd}${node.val}`;

    case nodeType.stringLiteral:
      return `${initialInd}"${escapeString(node.val)}"`;

    case nodeType.unaryExpr:
      return `${initialInd}${node.op}${serializeNode(node.expr, indent, pretty, false)}`;

    case nodeType.binaryExpr: {
      let matching = "";
      let grouping = "";
      let fill = "";
      const vm = node.matching;
      if (vm !== null) {
        if (
          vm.labels.length > 0 ||
          vm.on ||
          vm.card === vectorMatchCardinality.manyToOne ||
          vm.card === vectorMatchCardinality.oneToMany
        ) {
          matching = ` ${vm.on ? "on" : "ignoring"}(${labelNameList(vm.labels)})`;
        }

        if (
          vm.card === vectorMatchCardinality.manyToOne ||
          vm.card === vectorMatchCardinality.oneToMany
        ) {
          grouping = ` group_${vm.card === vectorMatchCardinality.manyToOne ? "left" : "right"}(${labelNameList(vm.include)})`;
        }

        const lfill = vm.fillValues.lhs;
        const rfill = vm.fillValues.rhs;
        if (lfill !== null || rfill !== null) {
          if (lfill === rfill) {
            fill = ` fill(${lfill})`;
          } else {
            if (lfill !== null) {
              fill += ` fill_left(${lfill})`;
            }
            if (rfill !== null) {
              fill += ` fill_right(${rfill})`;
            }
          }
        }
      }

      return `${serializeNode(maybeParenthesizeBinopChild(node.op, node.lhs), childIndent, pretty)}${childSeparator}${ind}${
        node.op
      }${node.bool ? " bool" : ""}${matching}${grouping}${fill}${childSeparator}${serializeNode(
        maybeParenthesizeBinopChild(node.op, node.rhs),
        childIndent,
        pretty
      )}`;
    }

    case nodeType.placeholder:
      // TODO: Should we just throw an error when trying to serialize an AST containing a placeholder node?
      // (that would currently break editing-as-text of ASTs that contain placeholders)
      return `${initialInd}…${
        node.children.length > 0
          ? `(${childListSeparator}${node.children
              .map((child) => serializeNode(child, childIndent, pretty))
              .join("," + childSeparator)}${childListSeparator}${ind})`
          : ""
      }`;

    default:
      throw new Error("unsupported node type");
  }
};

export default serializeNode;
