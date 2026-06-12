// Copyright The Prometheus Authors

import { ReactNode } from "react";
import { Text, List } from "@mantine/core";
import { DurationNode } from "./ast";
import { formatDurationNode } from "./format";
import { serializeDurationNode } from "./serialize";

// collectNotes walks a duration expression AST and accumulates explanatory
// notes for any special duration operators it encounters, deduplicating
// equivalent notes via the seen set.
export const collectNotes = (
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
  if ((node.op === "min_of" || node.op === "max_of") && node.lhs && node.rhs) {
    const key = `${node.op}:${serializeDurationNode(node.lhs)}:${serializeDurationNode(node.rhs)}`;
    if (!seen.has(key)) {
      seen.add(key);
      notes.push(
        <>
          <span className="promql-code promql-keyword">{node.op}()</span> returns
          the {node.op === "min_of" ? "smaller" : "larger"} of{" "}
          <span className="promql-code">{formatDurationNode(node.lhs)}</span> and{" "}
          <span className="promql-code">{formatDurationNode(node.rhs)}</span>.
        </>
      );
    }
  }
  collectNotes(node.lhs, notes, seen);
  collectNotes(node.rhs, notes, seen);
};

// durationExprNote renders the explanatory notes for a duration expression,
// or null if the expression has no notes worth explaining.
export const durationExprNote = (expr: DurationNode): ReactNode => {
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
      <List fz="sm" mt="xs" withPadding>
        {notes.map((note, i) => (
          <List.Item key={i}>{note}</List.Item>
        ))}
      </List>
    </Text>
  );
};
