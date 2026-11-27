// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import HistogramChart from "./HistogramChart";
import classes from "./HistogramChart.module.css";
import { Histogram } from "../../types/types";

describe.each(["linear", "exponential"])("HistogramChart (%s)", (scale) => {
  it("shows an error instead of invalid chart coordinates for malformed boundaries", () => {
    const html = renderToStaticMarkup(
      <MantineProvider env="test">
        <HistogramChart
          histogram={{ count: "1", sum: "0", buckets: [[0, "NaN", "1", "1"]] }}
          index={0}
          scale={scale}
        />
      </MantineProvider>,
    );
    const container = document.createElement("div");
    container.innerHTML = html;
    expect(container.querySelector('[role="alert"]')).toHaveTextContent(
      "Invalid histogram bucket boundaries or counts.",
    );
    expect(
      container.querySelector(`.${classes.histogramBucketSlot}`),
    ).toBeNull();
  });

  it.each([
    ["-0", "0", "5", "100%"],
    ["-0.001", "0.001", "5", "100%"],
    ["-0", "0", "0", "0%"],
  ])(
    "renders only [%s, %s] with %s observations",
    (left, right, count, height) => {
      const html = renderToStaticMarkup(
        <MantineProvider env="test">
          <HistogramChart
            histogram={{ count, sum: "0", buckets: [[3, left, right, count]] }}
            index={0}
            scale={scale}
          />
        </MantineProvider>,
      );

      expect(html).not.toMatch(/NaN|Infinity/);
      const container = document.createElement("div");
      container.innerHTML = html;
      expect(
        container.querySelector(`.${classes.histogramBucketSlot}`),
      ).toHaveStyle({ left: "0%", width: "100%" });
      expect(
        container.querySelector(`.${classes.histogramBucket}`),
      ).toHaveStyle({
        height,
      });
      expect(
        container.querySelectorAll(`.${classes.histogramXTick}`)[1],
      ).toHaveStyle({ left: "50%" });
    },
  );

  it("preserves the layout of negative, zero, and positive buckets", () => {
    const html = renderToStaticMarkup(
      <MantineProvider env="test">
        <HistogramChart
          histogram={{
            count: "10",
            sum: "0",
            buckets: [
              [1, "-2", "-1", "2"],
              [3, "-1", "1", "5"],
              [0, "1", "2", "3"],
            ],
          }}
          index={0}
          scale={scale}
        />
      </MantineProvider>,
    );

    expect(html).not.toMatch(/NaN|Infinity/);
    const container = document.createElement("div");
    container.innerHTML = html;
    const slots = container.querySelectorAll<HTMLElement>(
      `.${classes.histogramBucketSlot}`,
    );
    expect(slots).toHaveLength(3);
    const widths =
      scale === "linear" ? [25, 50, 25] : [100 / 3, 100 / 3, 100 / 3];
    const positions = scale === "linear" ? [0, 25, 75] : [0, 100 / 3, 200 / 3];
    slots.forEach((slot, i) => {
      expect(parseFloat(slot.style.width)).toBeCloseTo(widths[i]);
      expect(parseFloat(slot.style.left)).toBeCloseTo(positions[i]);
    });
  });

  it.each<NonNullable<Histogram["buckets"]>[]>([
    [
      [
        [1, "-Inf", "0", "5"],
        [0, "0", "10", "5"],
        [0, "10", "+Inf", "5"],
      ],
    ],
    [
      [
        [1, "-Inf", "-2", "5"],
        [1, "-2", "-1", "5"],
      ],
    ],
    [
      [
        [0, "1", "2", "5"],
        [0, "2", "+Inf", "5"],
      ],
    ],
    [
      [
        [0, "0", "1", "5"],
        [0, "1", "2", "5"],
      ],
    ],
    [
      [
        [1, "-2", "-1", "5"],
        [1, "-1", "0", "5"],
      ],
    ],
    [[[3, "-Inf", "+Inf", "5"]]],
  ])("renders finite, non-overlapping bars for %j", (buckets) => {
    const html = renderToStaticMarkup(
      <MantineProvider env="test">
        <HistogramChart
          histogram={{ count: "15", sum: "0", buckets }}
          index={0}
          scale={scale}
        />
      </MantineProvider>,
    );
    expect(html).not.toMatch(/NaN|Infinity/);
    const container = document.createElement("div");
    container.innerHTML = html;
    const slots = container.querySelectorAll<HTMLElement>(
      `.${classes.histogramBucketSlot}`,
    );
    expect(slots).toHaveLength(buckets.length);
    let previousRight = 0;
    slots.forEach((slot) => {
      const left = parseFloat(slot.style.left);
      const width = parseFloat(slot.style.width);
      expect(Number.isFinite(left)).toBe(true);
      expect(Number.isFinite(width)).toBe(true);
      expect(left).toBeGreaterThanOrEqual(previousRight - 1e-8);
      expect(width).toBeGreaterThanOrEqual(0);
      expect(left + width).toBeLessThanOrEqual(100 + 1e-8);
      previousRight = left + width;
    });
    if (buckets[0][1] === "-Inf") {
      expect(container).toHaveTextContent("-Inf");
    }
    if (buckets[buckets.length - 1][2] === "+Inf") {
      expect(container).toHaveTextContent("+Inf");
    }
  });

  it("keeps an asymmetric zero-crossing bucket in proportion around zero", () => {
    const html = renderToStaticMarkup(
      <MantineProvider env="test">
        <HistogramChart
          histogram={{
            count: "15",
            sum: "0",
            buckets: [
              [1, "-4", "-2", "5"],
              [3, "-2", "8", "5"],
              [0, "8", "16", "5"],
            ],
          }}
          index={0}
          scale={scale}
        />
      </MantineProvider>,
    );
    const container = document.createElement("div");
    container.innerHTML = html;
    const slots = container.querySelectorAll<HTMLElement>(
      `.${classes.histogramBucketSlot}`,
    );
    const left = parseFloat(slots[1].style.left);
    const right = left + parseFloat(slots[1].style.width);
    const zero = parseFloat(
      (
        container.querySelectorAll(
          `.${classes.histogramXTick}`,
        )[1] as HTMLElement
      ).style.left,
    );
    expect((zero - left) / (right - zero)).toBeCloseTo(2 / 8);
    expect(right).toBeCloseTo(parseFloat(slots[2].style.left));
  });
});
