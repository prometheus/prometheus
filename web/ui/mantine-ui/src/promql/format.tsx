import React, { ReactElement, ReactNode, JSX } from "react";
import ASTNode, {
  VectorSelector,
  matchType,
  vectorMatchCardinality,
  nodeType,
  StartOrEnd,
  MatrixSelector,
} from "./ast";
import { formatPrometheusDuration } from "../lib/formatTime";
import {
  maybeParenthesizeBinopChild,
  escapeString,
  maybeQuoteLabelName,
  metricContainsExtendedCharset,
} from "./utils";

export const labelNameList = (labels: string[]): React.ReactNode[] => {
  return labels.map((l, i) => {
    return (
      <span key={i}>
        {i !== 0 && ", "}
        <span className="promql-code promql-label-name">
          {maybeQuoteLabelName(l)}
        </span>
      </span>
    );
  });
};

const formatAtAndOffset = (
  timestamp: number | null,
  startOrEnd: StartOrEnd,
  offset: number
): ReactNode => (
  <>
    {timestamp !== null ? (
      <>
        {" "}
        <span className="promql-operator">@</span>{" "}
        <span className="promql-number">{(timestamp / 1000).toFixed(3)}</span>
      </>
    ) : startOrEnd !== null ? (
      <>
        {" "}
        <span className="promql-operator">@</span>{" "}
        <span className="promql-keyword">{startOrEnd}</span>
        <span className="promql-paren">(</span>
        <span className="promql-paren">)</span>
      </>
    ) : (
      <></>
    )}
    {offset === 0 ? (
      <></>
    ) : offset > 0 ? (
      <>
        {" "}
        <span className="promql-keyword">offset</span>{" "}
        <span className="promql-duration">
          {formatPrometheusDuration(offset)}
        </span>
      </>
    ) : (
      <>
        {" "}
        <span className="promql-keyword">offset</span>{" "}
        <span className="promql-duration">
          -{formatPrometheusDuration(-offset)}
        </span>
      </>
    )}
  </>
);

const formatSelector = (
  node: VectorSelector | MatrixSelector
): ReactElement => {
  const matchLabels: JSX.Element[] = [];

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
    matchLabels.push(
      <span key="__name__">
        <span className="promql-string">"{escapeString(metricName)}"</span>
      </span>
    );
  }

  matchLabels.push(
    ...node.matchers
      .filter((m) => !(m.name === "__name__" && m.type === matchType.equal))
      .map((m, i) => (
        <span key={i}>
          {(i !== 0 || metricExtendedCharset) && ","}
          <span className="promql-label-name">
            {maybeQuoteLabelName(m.name)}
          </span>
          {m.type}
          <span className="promql-string">"{escapeString(m.value)}"</span>
        </span>
      ))
  );

  return (
    <>
      {!metricExtendedCharset && (
        <span className="promql-metric-name">{metricName}</span>
      )}
      {matchLabels.length > 0 && (
        <>
          {"{"}
          <span className="promql-metric-name">{matchLabels}</span>
          {"}"}
        </>
      )}
      {node.type === nodeType.matrixSelector && (
        <>
          [
          <span className="promql-duration">
            {formatPrometheusDuration(node.range)}
          </span>
          ]
        </>
      )}
      {node.anchored && (
        <>
          {" "}
          <span className="promql-keyword">anchored</span>
        </>
      )}
      {node.smoothed && (
        <>
          {" "}
          <span className="promql-keyword">smoothed</span>
        </>
      )}
      {formatAtAndOffset(node.timestamp, node.startOrEnd, node.offset)}
    </>
  );
};

const ellipsis = <span className="promql-ellipsis">…</span>;

const formatNodeInternal = (
  node: ASTNode,
  showChildren: boolean,
  maxDepth?: number
): React.ReactNode => {
  if (maxDepth === 0) {
    return ellipsis;
  }

  const childMaxDepth = maxDepth === undefined ? undefined : maxDepth - 1;

  switch (node.type) {
    case nodeType.aggregation:
      return (
        <>
          <span className="promql-keyword">{node.op}</span>
          {node.without ? (
            <>
              {" "}
              <span className="promql-keyword">without</span>
              <span className="promql-paren">(</span>
              {labelNameList(node.grouping)}
              <span className="promql-paren">)</span>{" "}
            </>
          ) : (
            node.grouping.length > 0 && (
              <>
                {" "}
                <span className="promql-keyword">by</span>
                <span className="promql-paren">(</span>
                {labelNameList(node.grouping)}
                <span className="promql-paren">)</span>{" "}
              </>
            )
          )}
          {showChildren && (
            <>
              <span className="promql-paren">(</span>
              {node.param !== null && (
                <>{formatNode(node.param, showChildren, childMaxDepth)}, </>
              )}
              {formatNode(node.expr, showChildren, childMaxDepth)}
              <span className="promql-paren">)</span>
            </>
          )}
        </>
      );
    case nodeType.subquery:
      return (
        <>
          {showChildren && formatNode(node.expr, showChildren, childMaxDepth)}[
          <span className="promql-duration">
            {formatPrometheusDuration(node.range)}
          </span>
          :
          {node.step !== 0 && (
            <span className="promql-duration">
              {formatPrometheusDuration(node.step)}
            </span>
          )}
          ]{formatAtAndOffset(node.timestamp, node.startOrEnd, node.offset)}
        </>
      );
    case nodeType.parenExpr:
      return (
        <>
          <span className="promql-paren">(</span>
          {showChildren && formatNode(node.expr, showChildren, childMaxDepth)}
          <span className="promql-paren">)</span>
        </>
      );
    case nodeType.call: {
      const children =
        childMaxDepth === undefined || childMaxDepth > 0
          ? node.args.map((arg, i) => (
              <span key={i}>
                {i !== 0 && ", "}
                {formatNode(arg, showChildren)}
              </span>
            ))
          : node.args.length > 0
            ? ellipsis
            : "";

      return (
        <>
          <span className="promql-keyword">{node.func.name}</span>
          {showChildren && (
            <>
              <span className="promql-paren">(</span>
              {children}
              <span className="promql-paren">)</span>
            </>
          )}
        </>
      );
    }
    case nodeType.matrixSelector:
      return formatSelector(node);
    case nodeType.vectorSelector:
      return formatSelector(node);
    case nodeType.numberLiteral:
      return <span className="promql-number">{node.val}</span>;
    case nodeType.stringLiteral:
      return <span className="promql-string">"{escapeString(node.val)}"</span>;
    case nodeType.unaryExpr:
      return (
        <>
          <span className="promql-operator">{node.op}</span>
          {showChildren && formatNode(node.expr, showChildren, childMaxDepth)}
        </>
      );
    case nodeType.binaryExpr: {
      let matching = <></>;
      let grouping = <></>;
      let fill = <></>;
      const vm = node.matching;
      if (vm !== null) {
        if (
          vm.labels.length > 0 ||
          vm.on ||
          vm.card === vectorMatchCardinality.manyToOne ||
          vm.card === vectorMatchCardinality.oneToMany
        ) {
          matching = (
            <>
              {" "}
              <span className="promql-keyword">
                {vm.on ? "on" : "ignoring"}
              </span>
              <span className="promql-paren">(</span>
              {labelNameList(vm.labels)}
              <span className="promql-paren">)</span>
            </>
          );
        }

        if (
          vm.card === vectorMatchCardinality.manyToOne ||
          vm.card === vectorMatchCardinality.oneToMany
        ) {
          grouping = (
            <>
              <span className="promql-keyword">
                {" "}
                group_
                {vm.card === vectorMatchCardinality.manyToOne
                  ? "left"
                  : "right"}
              </span>
              <span className="promql-paren">(</span>
              {labelNameList(vm.include)}
              <span className="promql-paren">)</span>
            </>
          );
        }

        const lfill = vm.fillValues.lhs;
        const rfill = vm.fillValues.rhs;
        if (lfill !== null || rfill !== null) {
          if (lfill === rfill) {
            fill = (
              <>
                {" "}
                <span className="promql-keyword">fill</span>
                <span className="promql-paren">(</span>
                <span className="promql-number">{lfill}</span>
                <span className="promql-paren">)</span>
              </>
            );
          } else {
            fill = (
              <>
                {lfill !== null && (
                  <>
                    {" "}
                    <span className="promql-keyword">fill_left</span>
                    <span className="promql-paren">(</span>
                    <span className="promql-number">{lfill}</span>
                    <span className="promql-paren">)</span>
                  </>
                )}
                {rfill !== null && (
                  <>
                    {" "}
                    <span className="promql-keyword">fill_right</span>
                    <span className="promql-paren">(</span>
                    <span className="promql-number">{rfill}</span>
                    <span className="promql-paren">)</span>
                  </>
                )}
              </>
            );
          }
        }
      }

      return (
        <>
          {showChildren &&
            formatNode(
              maybeParenthesizeBinopChild(node.op, node.lhs),
              showChildren,
              childMaxDepth
            )}{" "}
          {["atan2", "and", "or", "unless"].includes(node.op) ? (
            <span className="promql-keyword">{node.op}</span>
          ) : (
            <span className="promql-operator">{node.op}</span>
          )}
          {node.bool && (
            <>
              {" "}
              <span className="promql-keyword">bool</span>
            </>
          )}
          {matching}
          {grouping}
          {fill}{" "}
          {showChildren &&
            formatNode(
              maybeParenthesizeBinopChild(node.op, node.rhs),
              showChildren,
              childMaxDepth
            )}
        </>
      );
    }
    case nodeType.placeholder:
      // TODO: Include possible children of placeholders somehow?
      return ellipsis;
    default:
      throw new Error("unsupported node type");
  }
};

export const formatNode = (
  node: ASTNode,
  showChildren: boolean,
  maxDepth?: number
): React.ReactElement => (
  <span className="promql-code">
    {formatNodeInternal(node, showChildren, maxDepth)}
  </span>
);
