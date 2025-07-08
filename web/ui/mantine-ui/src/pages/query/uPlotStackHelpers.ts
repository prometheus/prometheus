import { lighten } from "@mantine/core";
import uPlot, { AlignedData, TypedArray } from "uplot";

// Stacking code adapted from https://leeoniya.github.io/uPlot/demos/stack.js
function stack(
  data: uPlot.AlignedData,
  omit: (i: number) => boolean
): { data: uPlot.AlignedData; bands: uPlot.Band[] } {
  const data2: uPlot.AlignedData = [];
  let bands: uPlot.Band[] = [];
  const d0Len = data[0].length;
  const accum = Array(d0Len);

  for (let i = 0; i < d0Len; i++) {
    accum[i] = 0;
  }

  for (let i = 1; i < data.length; i++) {
    data2.push(
      (omit(i)
        ? data[i]
        : data[i].map((v, i) => (accum[i] += +(v || 0)))) as TypedArray
    );
  }

  for (let i = 1; i < data.length; i++) {
    if (!omit(i)) {
      bands.push({
        series: [data.findIndex((_s, j) => j > i && !omit(j)), i],
      });
    }
  }

  bands = bands.filter((b) => b.series[1] > -1);

  return {
    data: [data[0]].concat(data2) as AlignedData,
    bands,
  };
}

export function setStackedOpts(opts: uPlot.Options, data: uPlot.AlignedData) {
  const stacked = stack(data, (_i) => false);
  opts.bands = stacked.bands;

  opts.cursor = opts.cursor || {};
  opts.cursor.dataIdx = (_u, seriesIdx, closestIdx, _xValue) =>
    data[seriesIdx][closestIdx] == null ? null : closestIdx;

  opts.series.forEach((s) => {
    // s.value = (u, v, si, i) => data[si][i];

    s.points = s.points || {};

    if (s.stroke) {
      s.fill = lighten(s.stroke as string, 0.6);
    }

    // scan raw unstacked data to return only real points
    s.points.filter = (
      _self: uPlot,
      seriesIdx: number,
      show: boolean,
      _gaps?: null | number[][]
    ): number[] | null => {
      if (show) {
        const pts: number[] = [];
        data[seriesIdx].forEach((v, i) => {
          if (v != null) {
            pts.push(i);
          }
        });
        return pts;
      }
      return null;
    };
  });

  // force 0 to be the sum minimum this instead of the bottom series
  opts.scales = opts.scales || {};
  opts.scales.y = {
    range: (_u, _min, max) => {
      const minMax = uPlot.rangeNum(0, max, 0.1, true);
      return [0, minMax[1]];
    },
  };

  // restack on toggle
  opts.hooks = opts.hooks || {};
  opts.hooks.setSeries = opts.hooks.setSeries || [];
  opts.hooks.setSeries.push((u, _i) => {
    const stacked = stack(data, (i) => !u.series[i].show);
    u.delBand(null);
    stacked.bands.forEach((b) => u.addBand(b));
    u.setData(stacked.data);
  });

  return { opts, data: stacked.data };
}
