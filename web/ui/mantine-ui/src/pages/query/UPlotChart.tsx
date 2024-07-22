import { FC, useEffect, useState } from "react";
import { RangeSamples } from "../../api/responseTypes/query";
import classes from "./Graph.module.css";
import { GraphDisplayMode } from "../../state/queryPageSlice";
import { formatSeries } from "../../lib/formatSeries";
import uPlot, { Series } from "uplot";
import UplotReact from "uplot-react";
import "uplot/dist/uPlot.min.css";
import "./uplot.css";
import { formatTimestamp } from "../../lib/formatTime";
import { computePosition, shift, flip, offset } from "@floating-ui/dom";
import { colorPool } from "./ColorPool";
import { useSettings } from "../../state/settingsSlice";

const formatYAxisTickValue = (y: number | null): string => {
  if (y === null) {
    return "null";
  }
  const absY = Math.abs(y);

  if (absY >= 1e24) {
    return (y / 1e24).toFixed(2) + "Y";
  } else if (absY >= 1e21) {
    return (y / 1e21).toFixed(2) + "Z";
  } else if (absY >= 1e18) {
    return (y / 1e18).toFixed(2) + "E";
  } else if (absY >= 1e15) {
    return (y / 1e15).toFixed(2) + "P";
  } else if (absY >= 1e12) {
    return (y / 1e12).toFixed(2) + "T";
  } else if (absY >= 1e9) {
    return (y / 1e9).toFixed(2) + "G";
  } else if (absY >= 1e6) {
    return (y / 1e6).toFixed(2) + "M";
  } else if (absY >= 1e3) {
    return (y / 1e3).toFixed(2) + "k";
  } else if (absY >= 1) {
    return y.toFixed(2);
  } else if (absY === 0) {
    return y.toFixed(2);
  } else if (absY < 1e-23) {
    return (y / 1e-24).toFixed(2) + "y";
  } else if (absY < 1e-20) {
    return (y / 1e-21).toFixed(2) + "z";
  } else if (absY < 1e-17) {
    return (y / 1e-18).toFixed(2) + "a";
  } else if (absY < 1e-14) {
    return (y / 1e-15).toFixed(2) + "f";
  } else if (absY < 1e-11) {
    return (y / 1e-12).toFixed(2) + "p";
  } else if (absY < 1e-8) {
    return (y / 1e-9).toFixed(2) + "n";
  } else if (absY < 1e-5) {
    return (y / 1e-6).toFixed(2) + "µ";
  } else if (absY < 1e-2) {
    return (y / 1e-3).toFixed(2) + "m";
  } else if (absY <= 1) {
    return y.toFixed(2);
  }
  throw Error("couldn't format a value, this is a bug");
};

const escapeHTML = (str: string): string => {
  const entityMap: { [key: string]: string } = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
    "/": "&#x2F;",
  };

  return String(str).replace(/[&<>"'/]/g, function (s) {
    return entityMap[s];
  });
};

const formatLabels = (labels: { [key: string]: string }): string => `
            <div class="labels">
              ${Object.keys(labels).length === 0 ? '<div class="no-labels">no labels</div>' : ""}
              ${labels["__name__"] ? `<div><strong>${labels["__name__"]}</strong></div>` : ""}
              ${Object.keys(labels)
                .filter((k) => k !== "__name__")
                .map(
                  (k) =>
                    `<div><strong>${k}</strong>: ${escapeHTML(labels[k])}</div>`
                )
                .join("")}
            </div>`;

const tooltipPlugin = (useLocalTime: boolean) => {
  let over: HTMLDivElement;
  let boundingLeft: number;
  let boundingTop: number;
  let selectedSeriesIdx: number | null = null;

  const overlay = document.createElement("div");
  overlay.className = "u-tooltip";
  overlay.style.display = "none";

  return {
    hooks: {
      // Set up event handlers and append overlay.
      init: (u: uPlot) => {
        over = u.over;

        over.addEventListener("mouseenter", () => {
          overlay.style.display = "block";
        });

        over.addEventListener("mouseleave", () => {
          overlay.style.display = "none";
        });

        document.body.appendChild(overlay);
      },
      // When the chart is destroyed, remove the overlay from the DOM.
      destroy: () => {
        overlay.remove();
      },
      // When the chart is resized, store the bounding box of the overlay.
      setSize: () => {
        const bbox = over.getBoundingClientRect();
        boundingLeft = bbox.left;
        boundingTop = bbox.top;
      },
      // When a series is selected by hovering close to it, store the
      // index of the selected series.
      setSeries: (_u: uPlot, seriesIdx: number | null, _opts: Series) => {
        selectedSeriesIdx = seriesIdx;
      },
      // When the cursor is moved, update the tooltip with the current
      // series value and position it near the cursor.
      setCursor: (u: uPlot) => {
        const { left, top, idx } = u.cursor;

        if (
          idx === null ||
          idx === undefined ||
          left === null ||
          left === undefined ||
          top === null ||
          top === undefined ||
          selectedSeriesIdx === null
        ) {
          return;
        }

        const ts = u.data[0][idx];
        const value = u.data[selectedSeriesIdx][idx];
        const series = u.series[selectedSeriesIdx];
        // @ts-expect-error - uPlot doesn't have a field for labels, but we just attach some anyway.
        const labels = series.labels;
        if (typeof series.stroke !== "function") {
          throw new Error("series.stroke is not a function");
        }
        const color = series.stroke(u, selectedSeriesIdx);

        const x = left + boundingLeft;
        const y = top + boundingTop;

        // overlay.style.borderColor = color;

        // TODO: Use local time in formatTimestamp!
        overlay.innerHTML = `
            <div class="date">${formatTimestamp(ts, useLocalTime)}</div>
            <div class="series-value">
              <span class="detail-swatch" style="background-color: ${color}"></span>
              <span>${labels.__name__ ? labels.__name__ + ": " : " "}<strong>${value}</strong></span>
            </div>
            ${formatLabels(labels)}
          `.trimEnd();

        const virtualEl = {
          getBoundingClientRect() {
            return {
              width: 0,
              height: 0,
              x: x,
              y: y,
              left: x,
              right: x,
              top: y,
              bottom: y,
            };
          },
        };

        computePosition(virtualEl, overlay, {
          placement: "right-start",
          middleware: [offset(5), flip(), shift()],
        }).then(({ x, y }) => {
          Object.assign(overlay.style, {
            top: `${y}px`,
            left: `${x}px`,
          });
        });
      },
    },
  };
};

// A helper function to automatically create enough space for the Y axis
// ticket labels depending on their length.
const autoPadLeft = (
  u: uPlot,
  values: string[],
  axisIdx: number,
  cycleNum: number
) => {
  const axis = u.axes[axisIdx];

  // bail out, force convergence
  if (cycleNum > 1) {
    // @ts-expect-error - got this from a uPlot demo example, not sure if it's correct.
    return axis._size;
  }

  let axisSize = axis.ticks!.size! + axis.gap!;

  // Find longest tick text.
  const longestVal = (values ?? []).reduce(
    (acc, val) => (val.length > acc.length ? val : acc),
    ""
  );

  if (longestVal != "") {
    u.ctx.font = axis.font![0];
    axisSize += u.ctx.measureText(longestVal).width / devicePixelRatio;
  }

  return Math.ceil(axisSize);
};

const getOptions = (
  width: number,
  result: RangeSamples[],
  useLocalTime: boolean,
  onSelectRange: (_start: number, _end: number) => void
): uPlot.Options => ({
  width: width - 30,
  height: 550,
  // padding: [null, autoPadRight, null, null],
  cursor: {
    focus: {
      prox: 1000,
    },
    // Whether dragging on the chart should select a zoom area.
    drag: {
      x: true,
      // Don't zoom into the existing via uPlot, we want to load new (finger-grained) data instead.
      setScale: false,
    },
  },
  tzDate: useLocalTime
    ? undefined
    : (ts) => uPlot.tzDate(new Date(ts * 1e3), "Etc/UTC"),
  plugins: [tooltipPlugin(useLocalTime)],
  legend: {
    show: true,
    live: false,
    markers: {
      fill: (
        _u: uPlot,
        seriesIdx: number
      ): CSSStyleDeclaration["borderColor"] => {
        return colorPool[seriesIdx % colorPool.length];
      },
    },
  },
  // @ts-expect-error - uPlot enum types don't work across module boundaries,
  // see https://github.com/leeoniya/uPlot/issues/973.
  drawOrder: ["series", "axes"],
  focus: {
    alpha: 0.4,
  },
  axes: [
    // X axis (time).
    {
      labelSize: 20,
      stroke: "#333",
      grid: {
        show: false,
        stroke: "#eee",
        width: 2,
        dash: [],
      },
    },
    // Y axis (sample value).
    {
      values: (_u: uPlot, splits: number[]) => splits.map(formatYAxisTickValue),
      border: {
        show: true,
        stroke: "#333",
        width: 1,
      },
      labelGap: 8,
      labelSize: 8 + 12 + 8,
      stroke: "#333",
      size: autoPadLeft,
    },
  ],
  series: [
    {},
    ...result.map((r, idx) => ({
      label: formatSeries(r.metric),
      width: 2,
      labels: r.metric,
      stroke: colorPool[idx % colorPool.length],
    })),
  ],
  hooks: {
    setSelect: [
      (self: uPlot) => {
        onSelectRange(
          self.posToVal(self.select.left, "x"),
          self.posToVal(self.select.left + self.select.width, "x")
        );
      },
    ],
  },
});

export const normalizeData = (
  inputData: RangeSamples[],
  startTime: number,
  endTime: number,
  resolution: number
): uPlot.AlignedData => {
  const timeData: number[] = [];
  for (let t = startTime; t <= endTime; t += resolution) {
    timeData.push(t);
  }

  const values = inputData.map(({ values, histograms }) => {
    // Insert nulls for all missing steps.
    const data: (number | null)[] = [];
    let valuePos = 0;
    let histogramPos = 0;

    for (let t = startTime; t <= endTime; t += resolution) {
      // Allow for floating point inaccuracy.
      const currentValue = values && values[valuePos];
      const currentHistogram = histograms && histograms[histogramPos];
      if (
        currentValue &&
        values.length > valuePos &&
        currentValue[0] < t + resolution / 100
      ) {
        data.push(parseValue(currentValue[1]));
        valuePos++;
      } else if (
        currentHistogram &&
        histograms.length > histogramPos &&
        currentHistogram[0] < t + resolution / 100
      ) {
        data.push(parseValue(currentHistogram[1].sum));
        histogramPos++;
      } else {
        data.push(null);
      }
    }
    return data;
  });

  return [timeData, ...values];
};

const parseValue = (value: string): null | number => {
  const val = parseFloat(value);
  // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). They
  // can't be graphed, so show them as gaps (null).
  return isNaN(val) ? null : val;
};

export interface UPlotChartRange {
  startTime: number;
  endTime: number;
  resolution: number;
}

export interface UPlotChartProps {
  data: RangeSamples[];
  range: UPlotChartRange;
  width: number;
  showExemplars: boolean;
  displayMode: GraphDisplayMode;
  onSelectRange: (start: number, end: number) => void;
}

const UPlotChart: FC<UPlotChartProps> = ({
  data,
  range: { startTime, endTime, resolution },
  width,
  onSelectRange,
}) => {
  const [options, setOptions] = useState<uPlot.Options | null>(null);
  const { useLocalTime } = useSettings();

  useEffect(() => {
    if (width === 0) {
      return;
    }

    setOptions(getOptions(width, data, useLocalTime, onSelectRange));
  }, [width, data, useLocalTime, onSelectRange]);

  const seriesData: uPlot.AlignedData = normalizeData(
    data,
    startTime,
    endTime,
    resolution
  );
  // data[0].values?.map((v) => v[0]),
  // ...data.map((r) => r.values?.map((v) => parseFloat(v[1]))),
  // ...normalizeData(data, startTime, endTime, resolution),
  // ];

  if (options === null) {
    return;
  }

  return (
    <UplotReact
      options={options}
      data={seriesData}
      className={classes.uplotChart}
    />
  );
};

export default UPlotChart;
