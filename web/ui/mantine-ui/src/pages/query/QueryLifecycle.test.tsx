// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
import { MantineProvider } from "@mantine/core";
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Graph, { GraphProps } from "./Graph";
import TableTab from "./TableTab";
import { UPlotChartProps } from "./UPlotChart";
import { GraphDisplayMode } from "../../state/queryPageSlice";

const { visualizer } = vi.hoisted(() => ({
  visualizer: { endTime: null as number | null, range: 10000 },
}));
vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({
    pathPrefix: "/prometheus",
    showQueryWarnings: true,
    showQueryInfoNotices: true,
  }),
}));
vi.mock("../../state/hooks", () => ({
  useAppSelector: () => ({ visualizer }),
  useAppDispatch: () => vi.fn(),
}));
vi.mock("./TimeInput", () => ({ default: () => null }));
vi.mock("./QueryStatsDisplay", () => ({
  default: ({ responseTime }: { responseTime: number }) => (
    <span data-testid="duration">{responseTime}</span>
  ),
}));
vi.mock("./DataTable", () => ({
  default: ({
    limitResults,
    setLimitResults,
  }: {
    limitResults: boolean;
    setLimitResults: (limit: boolean) => void;
  }) => (
    <button onClick={() => setLimitResults(false)}>
      {limitResults ? "Limited results" : "All results"}
    </button>
  ),
}));
vi.mock("./UPlotChart", () => ({
  default: ({ range, data }: UPlotChartProps) => (
    <output data-testid="chart">{JSON.stringify({ range, data })}</output>
  ),
}));

let client: QueryClient;
let now: number;
const pending: {
  url: string;
  signal: AbortSignal;
  resolve: (response: unknown) => void;
}[] = [];
const wrapper = ({ children }: PropsWithChildren) => (
  <MantineProvider>
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  </MantineProvider>
);
const graphProps: GraphProps = {
  expr: "up",
  node: null,
  endTime: null,
  range: 10000,
  resolution: { type: "fixed", step: 1000 },
  showExemplars: false,
  displayMode: GraphDisplayMode.Lines,
  yAxisMin: null,
  retriggerIdx: 0,
  onSelectRange: vi.fn(),
};
const samples = [
  {
    metric: { __name__: "up" },
    values: [
      [90, "1"],
      [100, "1"],
    ],
  },
];
async function reply(index: number, table = false) {
  await act(async () =>
    pending[index].resolve({
      ok: true,
      json: async () => ({
        status: "success",
        data: {
          resultType: table ? "vector" : "matrix",
          result: samples,
          stats: {},
        },
      }),
    }),
  );
}
function chartRange() {
  return JSON.parse(screen.getByTestId("chart").textContent!).range;
}

beforeEach(() => {
  client = new QueryClient();
  now = 100000;
  visualizer.endTime = null;
  vi.spyOn(Date, "now").mockImplementation(() => now);
  vi.stubGlobal(
    "matchMedia",
    vi
      .fn()
      .mockReturnValue({
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
    vi.fn(
      (url, { signal }) =>
        new Promise((resolve) => pending.push({ url, signal, resolve })),
    ),
  );
});
afterEach(() => {
  cleanup();
  client.clear();
  onlineManager.setOnline(true);
  pending.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("graph query executions", () => {
  it("refreshes an identical result with its new range and retains the previous pair while loading", async () => {
    const { rerender } = render(<Graph {...graphProps} />, { wrapper });
    expect(
      new URL(pending[0].url, "http://localhost").searchParams.get("end"),
    ).toBe("100");
    await reply(0);
    await waitFor(() => expect(chartRange().endTime).toBe(100));
    now = 200000;
    rerender(<Graph {...graphProps} retriggerIdx={1} />);
    expect(chartRange().endTime).toBe(100);
    await reply(1);
    await waitFor(() => expect(chartRange().endTime).toBe(200));
    expect(chartRange().startTime).toBe(190);
    rerender(
      <Graph
        {...graphProps}
        retriggerIdx={1}
        yAxisMin={0}
        displayMode={GraphDisplayMode.Stacked}
        onSelectRange={vi.fn()}
      />,
    );
    expect(pending).toHaveLength(2);
  });

  it("cancels obsolete requests and resolves now only when an offline query resumes", async () => {
    onlineManager.setOnline(false);
    const { rerender } = render(<Graph {...graphProps} />, { wrapper });
    expect(pending).toHaveLength(0);
    now = 300000;
    act(() => onlineManager.setOnline(true));
    await waitFor(() => expect(pending).toHaveLength(1));
    expect(
      new URL(pending[0].url, "http://localhost").searchParams.get("end"),
    ).toBe("300");
    rerender(<Graph {...graphProps} endTime={500000} />);
    expect(pending[0].signal.aborted).toBe(true);
    await reply(1);
    await waitFor(() => expect(chartRange().endTime).toBe(500));
    await reply(0);
    expect(chartRange().endTime).toBe(500);
  });
});

describe("table query executions", () => {
  it("resets expanded results after repeated identical queries, including an offline execution", async () => {
    const { rerender } = render(
      <TableTab panelIdx={0} retriggerIdx={0} expr="up" />,
      { wrapper },
    );
    now += 25;
    await reply(0, true);
    fireEvent.click(
      await screen.findByRole("button", { name: "Limited results" }),
    );
    expect(
      screen.getByRole("button", { name: "All results" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("duration")).toHaveTextContent("25");
    onlineManager.setOnline(false);
    rerender(<TableTab panelIdx={0} retriggerIdx={1} expr="up" />);
    expect(pending).toHaveLength(1);
    now = 200000;
    act(() => onlineManager.setOnline(true));
    await waitFor(() => expect(pending).toHaveLength(2));
    expect(
      new URL(pending[1].url, "http://localhost").searchParams.get("time"),
    ).toBe("200");
    now += 50;
    await reply(1, true);
    expect(
      await screen.findByRole("button", { name: "Limited results" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("duration")).toHaveTextContent("50");
    visualizer.endTime = 400000;
    rerender(<TableTab panelIdx={0} retriggerIdx={1} expr="up" />);
    expect(
      new URL(pending[2].url, "http://localhost").searchParams.get("time"),
    ).toBe("400");
  });
});
