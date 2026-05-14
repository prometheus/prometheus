import { FC, ReactNode } from "react";
import {
  VectorSelector,
  MatrixSelector,
  nodeType,
  LabelMatcher,
  matchType,
  DurationNode,
} from "../../../promql/ast";
import { escapeString } from "../../../promql/utils";
import { useSuspenseAPIQuery } from "../../../api/api";
import { Card, Text, Divider, List } from "@mantine/core";
import { MetadataResult } from "../../../api/responseTypes/metadata";
import { formatPrometheusDuration } from "../../../lib/formatTime";
import { formatDurationNode } from "../../../promql/format";
import { useSettings } from "../../../state/settingsSlice";

const collectNotes = (
  node: DurationNode | null,
  notes: ReactNode[],
  seen: Set<string>
): void => {
  if (!node || node.type === "numberLiteral") {
    return;
  }
  if (node.op === "step") {
    if (!seen.has("step")) {
      seen.add("step");
      notes.push(
        <>
          <span className="promql-code promql-keyword">step()</span> resolves to
          the query step (the interval between data points in a range query, or
          the default evaluation step for instant queries).
        </>
      );
    }
    return;
  }
  if (node.op === "range") {
    if (!seen.has("range")) {
      seen.add("range");
      notes.push(
        <>
          <span className="promql-code promql-keyword">range()</span> resolves to
          the query range (the total time window covered by a range query).
        </>
      );
    }
    return;
  }
  if ((node.op === "min" || node.op === "max") && node.lhs && node.rhs) {
    const key = `${node.op}:${formatDurationNode(node.lhs)}:${formatDurationNode(node.rhs)}`;
    if (!seen.has(key)) {
      seen.add(key);
      notes.push(
        <>
          <span className="promql-code promql-keyword">{node.op}()</span> returns
          the {node.op === "min" ? "smaller" : "larger"} of{" "}
          <span className="promql-code">{formatDurationNode(node.lhs)}</span> and{" "}
          <span className="promql-code">{formatDurationNode(node.rhs)}</span>.
        </>
      );
    }
  }
  collectNotes(node.lhs, notes, seen);
  collectNotes(node.rhs, notes, seen);
};

const durationExprNote = (expr: DurationNode): ReactNode => {
  const notes: ReactNode[] = [];
  collectNotes(expr, notes, new Set<string>());
  if (notes.length === 0) {
    return null;
  }
  const exprCode = (
    <span className="promql-code">{formatDurationNode(expr)}</span>
  );
  if (notes.length === 1) {
    return (
      <Text fz="sm" mt="sm">
        In the duration expression {exprCode}, {notes[0]}
      </Text>
    );
  }
  return (
    <Text fz="sm" mt="sm">
      In the duration expression {exprCode}:
      <List mt="xs" withPadding>
        {notes.map((note, i) => (
          <List.Item key={i}>{note}</List.Item>
        ))}
      </List>
    </Text>
  );
};

interface SelectorExplainViewProps {
  node: VectorSelector | MatrixSelector;
}

const matchingCriteriaList = (
  name: string,
  matchers: LabelMatcher[]
): ReactNode => {
  return (
    <List fz="sm" my="md" withPadding>
      {name.length > 0 && (
        <List.Item>
          The metric name is{" "}
          <span className="promql-code promql-metric-name">{name}</span>.
        </List.Item>
      )}
      {matchers
        .filter((m) => !(m.name === "__name__"))
        .map((m) => {
          switch (m.type) {
            case matchType.equal:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  is exactly{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.notEqual:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  is not{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.matchRegexp:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  matches the regular expression{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            case matchType.matchNotRegexp:
              return (
                <List.Item>
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>
                  <span className="promql-code promql-operator">{m.type}</span>
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  : The label{" "}
                  <span className="promql-code promql-label-name">
                    {m.name}
                  </span>{" "}
                  does not match the regular expression{" "}
                  <span className="promql-code promql-string">
                    "{escapeString(m.value)}"
                  </span>
                  .
                </List.Item>
              );
            default:
              throw new Error("invalid matcher type");
          }
        })}
    </List>
  );
};

const SelectorExplainView: FC<SelectorExplainViewProps> = ({ node }) => {
  const baseMetricName = node.name.replace(/(_count|_sum|_bucket|_total)$/, "");
  const { lookbackDelta } = useSettings();

  // Try to get metadata for the full unchanged metric name first.
  const { data: fullMetricMeta } = useSuspenseAPIQuery<MetadataResult>({
    path: `/metadata`,
    params: {
      metric: node.name,
    },
  });

  // Also get prefix-stripped metric metadata in case the metadata only exists for
  // the histogram / summary base metric name.
  const { data: baseMetricMeta } = useSuspenseAPIQuery<MetadataResult>({
    path: `/metadata`,
    params: {
      metric: baseMetricName,
    },
  });

  // Determine which metadata to use.
  const metricMeta =
    fullMetricMeta.data[node.name] ?? baseMetricMeta.data[baseMetricName];

  return (
    <Card withBorder>
      <Text fz="lg" fw={600} mb="md">
        {node.type === nodeType.vectorSelector ? "Instant" : "Range"} vector
        selector
      </Text>
      <Text fz="sm">
        {metricMeta === undefined || metricMeta.length < 1 ? (
          <>No metric metadata found.</>
        ) : (
          <>
            <strong>Metric help</strong>: {metricMeta[0].help}
            <br />
            <strong>Metric type</strong>: {metricMeta[0].type}
          </>
        )}
      </Text>
      <Divider my="md" />
      <Text fz="sm">
        {node.type === nodeType.vectorSelector ? (
          <>
            This node {node.smoothed ? "smooths the value" : "selects the latest"}{" "}
            {node.anchored || node.smoothed ? "" : "non-stale "}
            {!node.smoothed && (
              <>
                sample value within the last{" "}
                <span className="promql-code promql-duration">{lookbackDelta}</span>
              </>
            )}
            {node.smoothed && (
              <>
                using <code>smoothed</code> mode (linear interpolation with nearest
                points within{" "}
                <span className="promql-code promql-duration">{lookbackDelta}</span>{" "}
                before and after execution timestamp, ignoring staleness markers)
              </>
            )}
          </>
        ) : (
          <>
            This node looks back{" "}
            {node.rangeExpr ? (
              <>
                a duration defined by{" "}
                <span className="promql-code">
                  {formatDurationNode(node.rangeExpr)}
                </span>
              </>
            ) : (
              <span className="promql-code">
                <span className="promql-duration">
                  {formatPrometheusDuration(node.range)}
                </span>
              </span>
            )}{" "}
            from the evaluation timestamp
            {node.anchored && (
              <>
                {" "}
                using <code>anchored</code> mode (includes first sample before or at
                start boundary, and last sample of the range at end boundary, ignoring staleness markers)
              </>
            )}
            {node.smoothed && (
              <>
                {" "}
                using <code>smoothed</code> mode (applies linear
                interpolation at the boundaries using nearest samples
                before and after boundaries, ignoring staleness markers)
              </>
            )}
          </>
        )}
        {node.timestamp !== null ? (
          <>
            , evaluated relative to an absolute evaluation timestamp of{" "}
            <span className="promql-number">
              {(node.timestamp / 1000).toFixed(3)}
            </span>
          </>
        ) : node.startOrEnd !== null ? (
          <>, evaluated relative to the {node.startOrEnd} of the query range</>
        ) : (
          <></>
        )}
        {node.offsetExpr ? (
          <>
            , time-shifted{" "}
            <span className="promql-code">
              {formatDurationNode(node.offsetExpr)}
            </span>
            {node.offset > 0
              ? " into the past"
              : node.offset < 0
                ? " into the future"
                : ""}
            ,
          </>
        ) : node.offset === 0 ? (
          <></>
        ) : node.offset > 0 ? (
          <>
            , time-shifted{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(node.offset)}
            </span>{" "}
            into the past,
          </>
        ) : (
          <>
            , time-shifted{" "}
            <span className="promql-code promql-duration">
              {formatPrometheusDuration(-node.offset)}
            </span>{" "}
            into the future,
          </>
        )}{" "}
        for any series that match all of the following criteria:
      </Text>
      {matchingCriteriaList(node.name, node.matchers)}
      <Text fz="sm">
        If a series has no values in that window
        {(node.offsetExpr || node.offset > 0) && (
          <>
            {" "}
            (relative to the time-shifted instant{" "}
            <span className="promql-code">
              {node.offsetExpr ? (
                formatDurationNode(node.offsetExpr)
              ) : (
                <span className="promql-duration">
                  {formatPrometheusDuration(node.offset)}
                </span>
              )}
            </span>{" "}
            in the past)
          </>
        )}
        , the series will not be returned.
      </Text>
      {node.type === nodeType.matrixSelector &&
        node.rangeExpr &&
        durationExprNote(node.rangeExpr)}
      {node.offsetExpr && durationExprNote(node.offsetExpr)}
    </Card>
  );
};

export default SelectorExplainView;
