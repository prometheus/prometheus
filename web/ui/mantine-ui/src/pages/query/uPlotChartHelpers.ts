import { RangeSamples } from "../../api/responseTypes/query";
import { formatSeries } from "../../lib/formatSeries";
import { formatTimestamp } from "../../lib/formatTime";
import { getSeriesColor } from "./colorPool";
import { computePosition, shift, flip, offset } from "@floating-ui/dom";
import uPlot, { AlignedData, Series } from "uplot";

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
              ${labels["__name__"] ? `<div><strong>${escapeHTML(labels["__name__"])}</strong></div>` : ""}
              ${Object.keys(labels)
                .filter((k) => k !== "__name__")
                .map(
                  (k) =>
                    `<div><strong>${escapeHTML(k)}</strong>: ${escapeHTML(labels[k])}</div>`
                )
                .join("")}
            </div>`;

const tooltipPlugin = (useLocalTime: boolean, data: AlignedData) => {
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
      // index of the selected series, so we can update the hover tooltip
      // in setCursor.
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
        const value = data[selectedSeriesIdx][idx];
        const series = u.series[selectedSeriesIdx];
        // @ts-expect-error - uPlot doesn't have a field for labels, but we just attach some anyway.
        const labels = series.labels;
        if (typeof series.stroke !== "function") {
          throw new Error("series.stroke is not a function");
        }
        const color = series.stroke(u, selectedSeriesIdx);

        const x = left + boundingLeft;
        const y = top + boundingTop;

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

// This filter functions ensures that only points that are disconnected
// from their neighbors are drawn. Otherwise, we just draw line segments
// without dots on them.
//
// Adapted from https://github.com/leeoniya/uPlot/blob/91de800538ee5d6f45f448d98b660a4a658e587b/demos/points.html#L15-L64
const onlyDrawPointsForDisconnectedSamplesFilter = (
  u: uPlot,
  seriesIdx: number,
  show: boolean,
  gaps?: null | number[][]
) => {
  const filtered = [];

  const series = u.series[seriesIdx];

  if (!show && gaps && gaps.length) {
    const [firstIdx, lastIdx] = series.idxs!;
    const xData = u.data[0];
    const yData = u.data[seriesIdx];
    const firstPos = Math.round(u.valToPos(xData[firstIdx], "x", true));
    const lastPos = Math.round(u.valToPos(xData[lastIdx], "x", true));

    if (gaps[0][0] === firstPos) {
      filtered.push(firstIdx);
    }

    // show single points between consecutive gaps that share end/start
    for (let i = 0; i < gaps.length; i++) {
      const thisGap = gaps[i];
      const nextGap = gaps[i + 1];

      if (nextGap && thisGap[1] === nextGap[0]) {
        // approx when data density is > 1pt/px, since gap start/end pixels are rounded
        let approxIdx = u.posToIdx(thisGap[1], true);

        if (yData[approxIdx] == null) {
          // scan left/right alternating to find closest index with non-null value
          for (let j = 1; j < 100; j++) {
            if (yData[approxIdx + j] != null) {
              approxIdx += j;
              break;
            }
            if (yData[approxIdx - j] != null) {
              approxIdx -= j;
              break;
            }
          }
        }

        filtered.push(approxIdx);
      }
    }

    if (gaps[gaps.length - 1][1] === lastPos) {
      filtered.push(lastIdx);
    }
  }

  return filtered.length ? filtered : null;
};

export const getUPlotOptions = (
  data: AlignedData,
  width: number,
  result: RangeSamples[],
  useLocalTime: boolean,
  yAxisMin: number | null,
  light: boolean,
  onSelectRange: (_start: number, _end: number) => void
): uPlot.Options => ({
  width: width - 30,
  height: 550,
  cursor: {
    focus: {
      prox: 1000,
    },
    // Whether dragging on the chart should select a zoom area.
    drag: {
      x: true,
      // Don't zoom into the existing data via uPlot. We want to load new
      // (finer-grained) data instead, which we do via a setSelect hook.
      setScale: false,
    },
  },
  tzDate: useLocalTime
    ? undefined
    : (ts) => uPlot.tzDate(new Date(ts * 1e3), "Etc/UTC"),
  plugins: [tooltipPlugin(useLocalTime, data)],
  legend: {
    show: true,
    live: false,
    isolate: true,
    markers: {
      fill: (
        _u: uPlot,
        seriesIdx: number
      ): CSSStyleDeclaration["borderColor"] =>
        // Because the index here is coming from uPlot, we need to subtract 1. Series 0
        // represents the X axis, so we need to skip it.
        getSeriesColor(seriesIdx - 1, light),
    },
  },
  // @ts-expect-error - uPlot enum types don't work across module boundaries,
  // see https://github.com/leeoniya/uPlot/issues/973.
  drawOrder: ["series", "axes"],
  focus: {
    alpha: 1,
  },
  scales:
    yAxisMin !== null
      ? {
          y: {
            range: (_u, _min, max) => {
              const minMax = uPlot.rangeNum(yAxisMin, max, 0.1, true);
              return [yAxisMin, minMax[1]];
            },
          },
        }
      : undefined,
  axes: [
    // X axis (time).
    {
      space: 80,
      labelSize: 20,
      stroke: light ? "#333" : "#eee",
      ticks: {
        stroke: light ? "#00000010" : "#ffffff20",
      },
      grid: {
        show: true,
        stroke: light ? "#00000010" : "#ffffff20",
        width: 2,
        dash: [],
      },
      values: [
        // See https://github.com/leeoniya/uPlot/tree/master/docs#axis--grid-opts and https://github.com/leeoniya/uPlot/issues/83.
        //
        // We want to achieve 24h-based time formatting instead of the default AM/PM-based time formatting.
        // We also want to render dates in an unambiguous format that uses the abbreviated month name instead of a US-centric DD/MM/YYYY format.
        //
        // The "tick incr" column defines the breakpoint in seconds at which the format changes.
        // The "default" column defines the default format for a tick at this breakpoint.
        // The "year"/"month"/"day"/"hour"/"min"/"sec" columns define additional values to display for year/month/day/... rollovers occurring around a tick.
        // The "mode" column value "1" means that rollover values will be concatenated with the default format (instead of replacing it).
        //
        // tick incr        default                  year                  month  day             hour   min    sec    mode
        // prettier-ignore
        [3600 * 24 * 365,   "{YYYY}",                null,                 null,  null,           null,  null,  null,     1],
        // prettier-ignore
        [3600 * 24 * 28,    "{MMM}",                 "\n{YYYY}",           null,  null,           null,  null,  null,     1],
        // prettier-ignore
        [3600 * 24,         "{MMM} {D}",             "\n{YYYY}",           null,  null,           null,  null,  null,     1],
        // prettier-ignore
        [3600,              "{HH}:{mm}",             "\n{MMM} {D} '{YY}",  null,  "\n{MMM} {D}",  null,  null,  null,     1],
        // prettier-ignore
        [60,                "{HH}:{mm}",             "\n{MMM} {D} '{YY}",  null,  "\n{MMM} {D}",  null,  null,  null,     1],
        // prettier-ignore
        [1,                 "{HH}:{mm}:{ss}",        "\n{MMM} {D} '{YY}",  null,  "\n{MMM} {D}",  null,  null,  null,     1],
        // prettier-ignore
        [0.001,             "{HH}:{mm}:{ss}.{fff}",  "\n{MMM} {D} '{YY}",  null,  "\n{MMM} {D}",  null,  null,  null,     1],
      ],
    },
    // Y axis (sample value).
    {
      values: (_u: uPlot, splits: number[]) => splits.map(formatYAxisTickValue),
      ticks: {
        stroke: light ? "#00000010" : "#ffffff20",
      },
      grid: {
        show: true,
        stroke: light ? "#00000010" : "#ffffff20",
        width: 2,
        dash: [],
      },
      labelGap: 8,
      labelSize: 8 + 12 + 8,
      stroke: light ? "#333" : "#eee",
      size: autoPadLeft,
    },
  ],
  series: [
    {},
    ...result.map(
      (r, idx): uPlot.Series => ({
        points: {
          filter: onlyDrawPointsForDisconnectedSamplesFilter,
        },
        label: formatSeries(r.metric),
        width: 1.5,
        // @ts-expect-error - uPlot doesn't have a field for labels, but we just attach some anyway.
        labels: r.metric,
        stroke: getSeriesColor(idx, light),
      })
    ),
  ],
  hooks: {
    setSelect: [
      (self: uPlot) => {
        // Disallow sub-second zoom as this cause inconsistenices in the X axis in uPlot.
        const leftVal = self.posToVal(self.select.left, "x");
        const rightVal = Math.max(
          self.posToVal(self.select.left + self.select.width, "x"),
          leftVal + 1
        );

        onSelectRange(leftVal, rightVal);
      },
    ],
  },
});

const parseValue = (value: string): null | number => {
  const val = parseFloat(value);
  // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). They
  // can't be graphed, so show them as gaps (null).
  return isNaN(val) ? null : val;
};

export const getUPlotData = (
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
    const data: (number | null)[] = [];
    let valuePos = 0;
    let histogramPos = 0;

    for (let t = startTime; t <= endTime; t += resolution) {
      const currentValue = values && values[valuePos];
      const currentHistogram = histograms && histograms[histogramPos];

      // Allow for floating point inaccuracy.
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
        // Insert nulls for all missing steps.
        data.push(null);
      }
    }
    return data;
  });

  return [timeData, ...values];
};
