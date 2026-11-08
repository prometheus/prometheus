// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import HistogramChart from "./HistogramChart";
import classes from "./HistogramChart.module.css";

describe.each(["linear", "exponential"])("HistogramChart (%s)", (scale) => {
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
});
