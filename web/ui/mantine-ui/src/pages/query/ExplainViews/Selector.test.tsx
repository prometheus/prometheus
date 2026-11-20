// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useSuspenseAPIQuery } from "../../../api/api";
import { LabelMatcher, matchType, nodeType } from "../../../promql/ast";
import SelectorExplainView from "./Selector";

vi.mock("../../../api/api", () => ({ useSuspenseAPIQuery: vi.fn() }));
vi.mock("../../../state/settingsSlice", () => ({
  useSettings: () => ({ lookbackDelta: "5m" }),
}));

// The parser leaves the selector's name empty when the metric name is set
// inside the braces, e.g. for {"http.server.requests", job="api"}.
const renderSelector = (...matchers: LabelMatcher[]) => {
  const meta = [{ type: "counter", help: "Total HTTP requests.", unit: "" }];
  const data = { data: { "http.server.requests": meta } };
  vi.mocked(useSuspenseAPIQuery).mockReturnValue({ data } as never);
  const mql = { addEventListener: vi.fn(), removeEventListener: vi.fn() };
  vi.stubGlobal("matchMedia", () => mql);
  const node = {
    type: nodeType.vectorSelector as const,
    name: "",
    matchers,
    offset: 0,
    offsetExpr: null,
    timestamp: null,
    startOrEnd: null,
    anchored: false,
    smoothed: false,
  };
  render(
    <MantineProvider env="test">
      <SelectorExplainView node={node} />
    </MantineProvider>,
  );
};

it("uses a metric name that is set inside the braces", () => {
  renderSelector(
    { type: matchType.equal, name: "__name__", value: "http.server.requests" },
    { type: matchType.equal, name: "job", value: "api" },
  );
  expect(useSuspenseAPIQuery).toHaveBeenCalledWith({
    path: "/metadata",
    params: { metric: "http.server.requests" },
  });
  expect(screen.getByText(/Total HTTP requests/)).toBeInTheDocument();
  expect(screen.getByText("http.server.requests")).toBeInTheDocument();
});

it("lists a regex matcher on __name__ as a criterion", () => {
  renderSelector(
    { type: matchType.matchRegexp, name: "__name__", value: "node_cpu.*" },
    { type: matchType.equal, name: "job", value: "node" },
  );
  expect(screen.getAllByText('"node_cpu.*"')).toHaveLength(2);
});
