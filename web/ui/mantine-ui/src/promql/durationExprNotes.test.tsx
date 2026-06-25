// Copyright The Prometheus Authors

import { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { DurationNode } from "./ast";
import { collectNotes } from "./durationExprNotes";

const durationLiteral = (seconds: number): DurationNode => ({
  type: "numberLiteral",
  val: `${seconds}`,
  duration: true,
});

const noteTexts = (node: DurationNode): string[] => {
  const notes: ReactNode[] = [];
  collectNotes(node, notes, new Set<string>());
  return notes.map((note) => render(<>{note}</>).container.textContent ?? "");
};

describe("collectNotes", () => {
  it("formats the full sub-expression so nested same-name operators stay distinct", () => {
    // foo[max_of(5m, max_of(2m, 3m))]: both notes use the max_of operator, so
    // each note must name its own sub-expression to be unambiguous.
    const expr: DurationNode = {
      type: "durationExpr",
      op: "max_of",
      lhs: durationLiteral(300),
      rhs: {
        type: "durationExpr",
        op: "max_of",
        lhs: durationLiteral(120),
        rhs: durationLiteral(180),
        wrapped: false,
      },
      wrapped: false,
    };

    expect(noteTexts(expr)).toEqual([
      "max_of(5m, max_of(2m, 3m)) returns the larger of 5m and max_of(2m, 3m).",
      "max_of(2m, 3m) returns the larger of 2m and 3m.",
    ]);
  });
});
