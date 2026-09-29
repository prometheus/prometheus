// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { cleanup, render, screen } from "@testing-library/react";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import QueryCostModal from "./QueryCostModal";

const query = vi.hoisted(() => vi.fn());

vi.mock("../../api/api", () => ({ useAPIQuery: query }));
vi.mock("../../state/hooks", () => ({
  useAppSelector: () => ({
    visualizer: {
      activeTab: "table",
      endTime: 1000000,
      range: 3600000,
      resolution: { type: "auto", density: "medium" },
    },
  }),
}));

beforeAll(() => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
});

afterEach(cleanup);
beforeEach(() => query.mockReset());

describe("QueryCostModal", () => {
  it("shows incomplete-estimate warnings alongside the estimated values", () => {
    const warning =
      "query cost estimate is incomplete: info() selects additional series at runtime";
    query.mockReturnValue({
      data: {
        data: { estimate: { seriesTouched: 1, samplesRead: 1 } },
        warnings: [warning],
      },
      error: null,
      isFetching: false,
    });

    render(
      <MantineProvider env="test">
        <QueryCostModal
          panelIdx={0}
          expr="info(metric)"
          opened
          onClose={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.getByText(warning)).toBeVisible();
    expect(screen.getByText("These are estimates")).toBeVisible();
    expect(
      screen.getByText(/higher or lower than the actual cost/),
    ).toBeVisible();
    expect(query).toHaveBeenCalledWith({
      path: "/query_cost",
      params: { query: "info(metric)", time: "1000" },
    });
  });

  it("does not request an estimate while closed", () => {
    render(
      <MantineProvider env="test">
        <QueryCostModal
          panelIdx={0}
          expr="metric"
          opened={false}
          onClose={vi.fn()}
        />
      </MantineProvider>,
    );
    expect(query).not.toHaveBeenCalled();
  });
});
