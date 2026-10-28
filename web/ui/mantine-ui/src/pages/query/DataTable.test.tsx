// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Histogram, RangeSamples } from "../../api/responseTypes/query";
import DataTable from "./DataTable";

vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({ useLocalTime: false }),
}));

beforeEach(() => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const histogram: Histogram = {
  count: "5",
  sum: "2.5",
  buckets: [[0, "0", "1", "5"]],
};
const emptyHistogram: Histogram = { count: "0", sum: "0" };
const manyBucketsHistogram: Histogram = {
  count: "125",
  sum: "7812.5",
  buckets: Array.from({ length: 125 }, (_, i) => [0, `${i}`, `${i + 1}`, "1"]),
};
const metric = { __name__: "request_duration_seconds" };

describe("DataTable", () => {
  it.each<{
    name: string;
    series: RangeSamples;
    expected: { timestamp: number; value: string | Histogram }[];
  }>([
    {
      name: "float-only",
      series: {
        metric,
        values: [
          [1572097950.93, "9"],
          [1572097965.931, "10"],
        ],
      },
      expected: [
        { timestamp: 1572097950.93, value: "9" },
        { timestamp: 1572097965.931, value: "10" },
      ],
    },
    {
      name: "histogram-only",
      series: {
        metric,
        histograms: [[1572097950.93, histogram]],
      },
      expected: [{ timestamp: 1572097950.93, value: histogram }],
    },
    {
      name: "mixed floats and histograms",
      series: {
        metric,
        values: [
          [1572097965.931, "9"],
          [1572097995.933, "10"],
        ],
        histograms: [
          [1572097950.93, histogram],
          [1572097980.932, emptyHistogram],
        ],
      },
      expected: [
        { timestamp: 1572097950.93, value: histogram },
        { timestamp: 1572097965.931, value: "9" },
        { timestamp: 1572097980.932, value: emptyHistogram },
        { timestamp: 1572097995.933, value: "10" },
      ],
    },
    {
      name: "histogram without buckets and an empty float array",
      series: {
        metric,
        values: [],
        histograms: [[1572097950.93, emptyHistogram]],
      },
      expected: [{ timestamp: 1572097950.93, value: emptyHistogram }],
    },
    {
      name: "histogram with many buckets",
      series: {
        metric,
        histograms: [[1572097950.93, manyBucketsHistogram]],
      },
      expected: [{ timestamp: 1572097950.93, value: manyBucketsHistogram }],
    },
  ])(
    "renders $name range samples in timestamp order",
    ({ series, expected }) => {
      render(
        <MantineProvider env="test">
          <DataTable
            data={{ resultType: "matrix", result: [series] }}
            limitResults={true}
            setLimitResults={vi.fn()}
          />
        </MantineProvider>
      );

      expect(screen.getByText(metric.__name__)).toBeInTheDocument();
      expect(screen.getAllByRole("table")).toHaveLength(1);
      const timestamps = screen.getAllByText(/^@ /);
      expect(timestamps.map((el) => el.textContent)).toEqual(
        expected.map(({ timestamp }) => `@ ${timestamp}`)
      );
      expected.forEach(({ timestamp, value }, index) => {
        expect(timestamps[index]).toHaveAttribute(
          "title",
          new Date(timestamp * 1000).toISOString().replace(/\.\d{3}Z$/, "Z")
        );
        const sample = timestamps[index].parentElement!;
        if (typeof value === "string") {
          expect(sample).toHaveTextContent(`${value} @ ${timestamp}`);
        } else {
          expect(sample).toHaveTextContent(
            `Buckets: ${value.buckets?.length ?? 0} Count: ${value.count} Sum: ${value.sum} @ ${timestamp}`
          );
        }
      });
    }
  );
});
