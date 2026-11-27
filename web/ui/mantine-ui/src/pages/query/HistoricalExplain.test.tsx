// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { MantineProvider } from "@mantine/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import QueryPanel from "./QueryPanel";
import { newDefaultPanel, Panel } from "../../state/queryPageSlice";
import { BinaryExpr, binaryOperatorType, nodeType } from "../../promql/ast";

vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "/prometheus", useLocalTime: false }),
}));
vi.mock("../../state/hooks", () => ({
  useAppSelector: (
    select: (state: { queryPage: { panels: Panel[] } }) => unknown,
  ) => select({ queryPage: { panels: [panel] } }),
  useAppDispatch: () => vi.fn(),
}));
vi.mock("./ExpressionInput", () => ({ default: () => null }));
vi.mock("./TimeInput", () => ({ default: () => null }));
vi.mock("./Graph", () => ({ default: () => null }));

let panel: Panel;
let client: QueryClient;
const requests: URL[] = [];
const historicalTimeMs = 1700000000000;
const currentTimeMs = historicalTimeMs + 600000;
const queueDepths = new Map([
  [0, 6],
  [historicalTimeMs / 1000, 4],
  [(historicalTimeMs + 123) / 1000, 4],
  [(historicalTimeMs + 60000) / 1000, 14],
]);
const node: BinaryExpr = {
  type: nodeType.binaryExpr,
  op: binaryOperatorType.add,
  lhs: {
    type: nodeType.vectorSelector,
    name: "demo_queue_depth",
    matchers: [],
    offset: 0,
    offsetExpr: null,
    timestamp: null,
    startOrEnd: null,
    anchored: false,
    smoothed: false,
  },
  rhs: { type: nodeType.numberLiteral, val: "1" },
  matching: null,
  bool: false,
};
const wrapper = ({ children }: PropsWithChildren) => (
  <MantineProvider env="test">
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  </MantineProvider>
);

beforeEach(() => {
  panel = newDefaultPanel();
  panel.expr = "demo_queue_depth + 1";
  panel.showTree = true;
  panel.visualizer.activeTab = "explain";
  client = new QueryClient();
  vi.spyOn(Date, "now").mockReturnValue(currentTimeMs);
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const url = new URL(input, "http://localhost");
      requests.push(url);
      const query = url.searchParams.get("query");
      const timestamp = Number(
        url.searchParams.get("time") ?? currentTimeMs / 1000,
      );
      const queueDepth = queueDepths.get(timestamp) ?? 99;
      const data = url.pathname.endsWith("/parse_query")
        ? node
        : query === "1"
          ? { resultType: "scalar", result: [timestamp, "1"] }
          : {
              resultType: "vector",
              result: [
                {
                  metric: { __name__: "demo_queue_depth", queue: "default" },
                  value: [
                    timestamp,
                    String(
                      query === "demo_queue_depth"
                        ? queueDepth
                        : queueDepth + 1,
                    ),
                  ],
                },
              ],
            };
      return { ok: true, json: async () => ({ status: "success", data }) };
    }),
  );
});
afterEach(() => {
  cleanup();
  client.clear();
  requests.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("historical query explanations", () => {
  it.each([
    { name: "current time", endTime: null, expected: "100" },
    { name: "historical time", endTime: historicalTimeMs, expected: "5" },
    {
      name: "fractional seconds",
      endTime: historicalTimeMs + 123,
      expected: "5",
    },
    { name: "Unix epoch", endTime: 0, expected: "7" },
  ])(
    "evaluates Explain and every tree node at $name",
    async ({ endTime, expected }) => {
      panel.visualizer.endTime = endTime;
      render(<QueryPanel idx={0} metricNames={["demo_queue_depth"]} />, {
        wrapper,
      });
      await waitFor(() => {
        const cells = screen.getAllByRole("cell");
        expect(cells[cells.length - 1].textContent).toBe(expected);
        expect(screen.getAllByText(/1 result/)).toHaveLength(3);
        expect(
          requests.some((url) => url.searchParams.get("query") === panel.expr),
        ).toBe(true);
      });
      for (const url of requests.filter((url) =>
        url.pathname.endsWith("/query"),
      )) {
        expect(url.searchParams.get("time")).toBe(
          endTime === null ? null : String(endTime / 1000),
        );
      }
    },
  );

  it("refreshes Explain and the tree when changing the selected time or returning to now", async () => {
    panel.visualizer.endTime = historicalTimeMs;
    const { rerender } = render(
      <QueryPanel idx={0} metricNames={["demo_queue_depth"]} />,
      { wrapper },
    );
    for (const { endTime, expected } of [
      { endTime: historicalTimeMs, expected: "5" },
      { endTime: historicalTimeMs + 60000, expected: "15" },
      { endTime: null, expected: "100" },
    ]) {
      panel.visualizer.endTime = endTime;
      rerender(<QueryPanel idx={0} metricNames={["demo_queue_depth"]} />);
      await waitFor(() => {
        const cells = screen.getAllByRole("cell");
        expect(cells[cells.length - 1].textContent).toBe(expected);
        expect(screen.getAllByText(/1 result/)).toHaveLength(3);
        expect(
          requests.some(
            (url) =>
              url.searchParams.get("query") === panel.expr &&
              url.searchParams.get("time") ===
                (endTime === null ? null : String(endTime / 1000)),
          ),
        ).toBe(true);
      });
      requests.length = 0;
    }
  });
});
