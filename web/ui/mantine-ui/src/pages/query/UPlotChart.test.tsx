// Copyright The Prometheus Authors

import { StrictMode } from "react";
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import uPlot from "uplot";
import { computePosition } from "@floating-ui/dom";
import UPlotChart, { UPlotChartProps } from "./UPlotChart";
import { getUPlotOptions } from "./uPlotChartHelpers";
import { GraphDisplayMode } from "../../state/queryPageSlice";

const { charts, settings } = vi.hoisted(() => ({
  charts: [] as {
    options: uPlot.Options;
    data: uPlot.AlignedData;
    destroy: ReturnType<typeof vi.fn>;
  }[],
  settings: { theme: "light", useLocalTime: false },
}));
vi.mock("../../state/settingsSlice", () => ({ useSettings: () => settings }));
vi.mock("@mantine/core", async (original) => ({
  ...(await original<typeof import("@mantine/core")>()),
  useComputedColorScheme: () => settings.theme,
  Text: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}));
vi.mock("uplot", () => ({
  default: vi.fn(function (options: uPlot.Options, data: uPlot.AlignedData) {
    expect(options.series).toHaveLength(data.length);
    const chart = { options, data, destroy: vi.fn() };
    charts.push(chart);
    return chart;
  }),
}));
vi.mock("@floating-ui/dom", async (original) => ({
  ...(await original<typeof import("@floating-ui/dom")>()),
  computePosition: vi.fn(),
}));

afterEach(() => {
  cleanup();
  charts.length = 0;
  settings.theme = "light";
  settings.useLocalTime = false;
  vi.clearAllMocks();
});

const props: UPlotChartProps = {
  data: [
    {
      metric: { __name__: "up" },
      values: [
        [0, "1"],
        [10, "2"],
      ],
    },
  ],
  range: { startTime: 0, endTime: 10, resolution: 10 },
  width: 800,
  showExemplars: false,
  displayMode: GraphDisplayMode.Lines,
  yAxisMin: null,
  onSelectRange: vi.fn(),
};

function runHook<A extends unknown[]>(
  hook:
    ((...args: A) => void) | (((...args: A) => void) | undefined)[] | undefined,
  ...args: A
) {
  for (const callback of Array.isArray(hook) ? hook : [hook])
    callback?.(...args);
}

describe("uPlot lifecycle", () => {
  it("constructs matching data and options through series, range, width, and theme changes", () => {
    const { rerender, unmount } = render(
      <StrictMode>
        <UPlotChart {...props} />
      </StrictMode>,
    );
    expect(charts).toHaveLength(2);
    expect(charts[0].destroy).toHaveBeenCalledTimes(1);
    const data = [
      ...props.data,
      {
        metric: { job: "second" },
        values: [
          [0, "3"],
          [10, "4"],
        ] as [number, string][],
      },
    ];
    rerender(
      <StrictMode>
        <UPlotChart {...props} data={data} />
      </StrictMode>,
    );
    expect(charts[charts.length - 1].data).toEqual([
      [0, 10],
      [1, 2],
      [3, 4],
    ]);
    rerender(
      <StrictMode>
        <UPlotChart
          {...props}
          data={data}
          range={{ startTime: 10, endTime: 20, resolution: 10 }}
        />
      </StrictMode>,
    );
    expect(charts[charts.length - 1].data[0]).toEqual([10, 20]);
    settings.theme = "dark";
    settings.useLocalTime = true;
    rerender(
      <StrictMode>
        <UPlotChart {...props} width={600} />
      </StrictMode>,
    );
    expect(charts[charts.length - 1].options.width).toBe(570);
    expect(charts[charts.length - 1].options.axes![0].stroke).toBe("#eee");
    expect(charts[charts.length - 1].options.tzDate).toBeUndefined();
    rerender(
      <StrictMode>
        <UPlotChart {...props} width={0} />
      </StrictMode>,
    );
    expect(charts.every((chart) => chart.destroy.mock.calls.length === 1)).toBe(
      true,
    );
    rerender(
      <StrictMode>
        <UPlotChart {...props} />
      </StrictMode>,
    );
    unmount();
    expect(charts.every((chart) => chart.destroy.mock.calls.length === 1)).toBe(
      true,
    );
  });

  it("stacks fresh data without modifying input samples and preserves zoom callbacks", () => {
    const data = [
      ...props.data,
      {
        metric: {},
        values: [
          [0, "3"],
          [10, "4"],
        ] as [number, string][],
      },
    ];
    const before = structuredClone(data);
    render(
      <UPlotChart
        {...props}
        data={data}
        displayMode={GraphDisplayMode.Stacked}
      />,
    );
    const chart = charts[0];
    expect(chart.data).toEqual([
      [0, 10],
      [1, 2],
      [4, 6],
    ]);
    expect(data).toEqual(before);
    const selection = {
      select: { left: 10, width: 30 },
      posToVal: (pos: number) => pos * 2,
    } as unknown as uPlot;
    runHook(chart.options.hooks!.setSelect, selection);
    expect(props.onSelectRange).toHaveBeenCalledWith(20, 80);
  });

  it("allocates tooltips only on init and cleans up each instance and pending positioning", async () => {
    const data: uPlot.AlignedData = [[0], [1]];
    const options = getUPlotOptions(
      data,
      800,
      props.data,
      false,
      null,
      true,
      vi.fn(),
    );
    expect(document.querySelectorAll(".u-tooltip")).toHaveLength(0);
    const hooks = options.plugins![0].hooks!;
    const makeChart = () =>
      ({
        over: document.createElement("div"),
        cursor: { left: 1, top: 2, idx: 0 },
        data,
        series: [{}, { labels: { __name__: "up" }, stroke: () => "red" }],
      }) as unknown as uPlot;
    const first = makeChart();
    const second = makeChart();
    runHook(hooks.init, first, options, data);
    runHook(hooks.init, second, options, data);
    const overlays = document.querySelectorAll<HTMLDivElement>(".u-tooltip");
    expect(overlays).toHaveLength(2);
    first.over.dispatchEvent(new Event("mouseenter"));
    expect(overlays[0].style.display).toBe("block");
    expect(overlays[1].style.display).toBe("none");
    let finish!: (result: Awaited<ReturnType<typeof computePosition>>) => void;
    vi.mocked(computePosition).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    runHook(hooks.setSeries, first, 1, {});
    runHook(hooks.setCursor, first);
    expect(overlays[0].textContent).toContain("up");
    runHook(hooks.destroy, first);
    first.over.dispatchEvent(new Event("mouseleave"));
    expect(overlays[0].style.display).toBe("block");
    await act(async () =>
      finish({
        x: 99,
        y: 99,
        placement: "right-start",
        strategy: "absolute",
        middlewareData: {},
      }),
    );
    expect(overlays[0].style.left).toBe("");
    expect(document.querySelectorAll(".u-tooltip")).toHaveLength(1);
    runHook(hooks.destroy, second);
    expect(document.querySelectorAll(".u-tooltip")).toHaveLength(0);
  });
});
