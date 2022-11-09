import $ from 'jquery';

import { escapeHTML } from '../../utils';
import { GraphProps, GraphData, GraphSeries, GraphExemplar } from './Graph';
import moment from 'moment-timezone';
import { colorPool } from './ColorPool';

export const formatValue = (y: number | null): string => {
  if (y === null) {
    return 'null';
  }
  const absY = Math.abs(y);

  if (absY >= 1e24) {
    return (y / 1e24).toFixed(2) + 'Y';
  } else if (absY >= 1e21) {
    return (y / 1e21).toFixed(2) + 'Z';
  } else if (absY >= 1e18) {
    return (y / 1e18).toFixed(2) + 'E';
  } else if (absY >= 1e15) {
    return (y / 1e15).toFixed(2) + 'P';
  } else if (absY >= 1e12) {
    return (y / 1e12).toFixed(2) + 'T';
  } else if (absY >= 1e9) {
    return (y / 1e9).toFixed(2) + 'G';
  } else if (absY >= 1e6) {
    return (y / 1e6).toFixed(2) + 'M';
  } else if (absY >= 1e3) {
    return (y / 1e3).toFixed(2) + 'k';
  } else if (absY >= 1) {
    return y.toFixed(2);
  } else if (absY === 0) {
    return y.toFixed(2);
  } else if (absY < 1e-23) {
    return (y / 1e-24).toFixed(2) + 'y';
  } else if (absY < 1e-20) {
    return (y / 1e-21).toFixed(2) + 'z';
  } else if (absY < 1e-17) {
    return (y / 1e-18).toFixed(2) + 'a';
  } else if (absY < 1e-14) {
    return (y / 1e-15).toFixed(2) + 'f';
  } else if (absY < 1e-11) {
    return (y / 1e-12).toFixed(2) + 'p';
  } else if (absY < 1e-8) {
    return (y / 1e-9).toFixed(2) + 'n';
  } else if (absY < 1e-5) {
    return (y / 1e-6).toFixed(2) + 'µ';
  } else if (absY < 1e-2) {
    return (y / 1e-3).toFixed(2) + 'm';
  } else if (absY <= 1) {
    return y.toFixed(2);
  }
  throw Error("couldn't format a value, this is a bug");
};

export const getHoverColor = (color: string, opacity: number, stacked: boolean): string => {
  const { r, g, b } = $.color.parse(color);
  if (!stacked) {
    return `rgba(${r}, ${g}, ${b}, ${opacity})`;
  }
  /*
    Unfortunately flot doesn't take into consideration
    the alpha value when adjusting the color on the stacked series.
    TODO: find better way to set the opacity.
  */
  const base = (1 - opacity) * 255;
  return `rgb(${Math.round(base + opacity * r)},${Math.round(base + opacity * g)},${Math.round(base + opacity * b)})`;
};

export const toHoverColor =
  (index: number, stacked: boolean) =>
  (
    series: GraphSeries,
    i: number
  ): { color: string; data: (number | null)[][]; index: number; labels: { [p: string]: string } } => ({
    ...series,
    color: getHoverColor(series.color, i !== index ? 0.3 : 1, stacked),
  });

export const getOptions = (stacked: boolean, useLocalTime: boolean): jquery.flot.plotOptions => {
  return {
    grid: {
      hoverable: true,
      clickable: true,
      autoHighlight: true,
      mouseActiveRadius: 100,
    },
    legend: {
      show: false,
    },
    xaxis: {
      mode: 'time',
      showTicks: true,
      showMinorTicks: true,
      timeBase: 'milliseconds',
      timezone: useLocalTime ? 'browser' : undefined,
    },
    yaxis: {
      tickFormatter: formatValue,
    },
    crosshair: {
      mode: 'xy',
      color: '#bbb',
    },
    tooltip: {
      show: true,
      cssClass: 'graph-tooltip',
      content: (_, xval, yval, { series }): string => {
        const both = series as GraphExemplar | GraphSeries;
        const { labels, color } = both;
        let dateTime = moment(xval);
        if (!useLocalTime) {
          dateTime = dateTime.utc();
        }

        const formatLabels = (labels: { [key: string]: string }): string => `
            <div class="labels">
              ${Object.keys(labels).length === 0 ? '<div class="mb-1 font-italic">no labels</div>' : ''}
              ${labels['__name__'] ? `<div class="mb-1"><strong>${labels['__name__']}</strong></div>` : ''}
              ${Object.keys(labels)
                .filter((k) => k !== '__name__')
                .map((k) => `<div class="mb-1"><strong>${k}</strong>: ${escapeHTML(labels[k])}</div>`)
                .join('')}
            </div>`;

        return `
            <div class="date">${dateTime.format('YYYY-MM-DD HH:mm:ss Z')}</div>
            <div>
              <span class="detail-swatch" style="background-color: ${color}"></span>
              <span>${labels.__name__ || 'value'}: <strong>${yval}</strong></span>
            </div>
            <div class="mt-2 mb-1 font-weight-bold">${'seriesLabels' in both ? 'Trace exemplar:' : 'Series:'}</div>
            ${formatLabels(labels)}
            ${
              'seriesLabels' in both
                ? `
            <div class="mt-2 mb-1 font-weight-bold">Associated series:</div>${formatLabels(both.seriesLabels)}
`
                : ''
            }
          `.trimEnd();
      },
      defaultTheme: false,
      lines: true,
    },
    series: {
      stack: false, // Stacking is set on a per-series basis because exemplar symbols don't support it.
      lines: {
        lineWidth: stacked ? 1 : 2,
        steps: false,
        fill: stacked,
      },
      shadowSize: 0,
    },
    selection: {
      mode: 'x',
    },
  };
};

export const normalizeData = ({ queryParams, data, exemplars, stacked }: GraphProps): GraphData => {
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  const { startTime, endTime, resolution } = queryParams!;

  let sum = 0;
  const values: number[] = [];
  // Exemplars are grouped into buckets by time to use for de-densifying.
  const buckets: { [time: number]: GraphExemplar[] } = {};
  for (const exemplar of exemplars || []) {
    for (const { labels, value, timestamp } of exemplar.exemplars) {
      const parsed = parseValue(value) || 0;
      sum += parsed;
      values.push(parsed);

      const bucketTime = Math.floor((timestamp / ((endTime - startTime) / 60)) * 0.8) * 1000;
      if (!buckets[bucketTime]) {
        buckets[bucketTime] = [];
      }

      buckets[bucketTime].push({
        seriesLabels: exemplar.seriesLabels,
        labels: labels,
        data: [[timestamp * 1000, parsed]],
        points: { symbol: exemplarSymbol },
        color: '#0275d8',
      });
    }
  }
  const deviation = stdDeviation(sum, values);

  return {
    series: data.result.map(({ values, histograms, metric }, index) => {
      // Insert nulls for all missing steps.
      const data = [];
      let valuePos = 0;
      let histogramPos = 0;

      for (let t = startTime; t <= endTime; t += resolution) {
        // Allow for floating point inaccuracy.
        const currentValue = values && values[valuePos];
        const currentHistogram = histograms && histograms[histogramPos];
        if (currentValue && values.length > valuePos && currentValue[0] < t + resolution / 100) {
          data.push([currentValue[0] * 1000, parseValue(currentValue[1])]);
          valuePos++;
        } else if (currentHistogram && histograms.length > histogramPos && currentHistogram[0] < t + resolution / 100) {
          data.push([currentHistogram[0] * 1000, parseValue(currentHistogram[1].sum)]);
          histogramPos++;
        } else {
          data.push([t * 1000, null]);
        }
      }

      return {
        labels: metric !== null ? metric : {},
        color: colorPool[index % colorPool.length],
        stack: stacked,
        data,
        index,
      };
    }),
    exemplars: Object.values(buckets).flatMap((bucket) => {
      if (bucket.length === 1) {
        return bucket[0];
      }
      return bucket
        .sort((a, b) => exValue(b) - exValue(a)) // Sort exemplars by value in descending order.
        .reduce((exemplars: GraphExemplar[], exemplar) => {
          if (exemplars.length === 0) {
            exemplars.push(exemplar);
          } else {
            const prev = exemplars[exemplars.length - 1];
            // Don't plot this exemplar if it's less than two times the standard
            // deviation spaced from the last.
            if (exValue(prev) - exValue(exemplar) >= 2 * deviation) {
              exemplars.push(exemplar);
            }
          }
          return exemplars;
        }, []);
    }),
  };
};

export const parseValue = (value: string): null | number => {
  const val = parseFloat(value);
  // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). They
  // can't be graphed, so show them as gaps (null).
  return isNaN(val) ? null : val;
};

const exemplarSymbol = (ctx: CanvasRenderingContext2D, x: number, y: number) => {
  // Center the symbol on the point.
  y = y - 3.5;

  // Correct if the symbol is overflowing off the grid.
  if (x > ctx.canvas.clientWidth - 59) {
    x = ctx.canvas.clientWidth - 59;
  }
  if (y > ctx.canvas.clientHeight - 40) {
    y = ctx.canvas.clientHeight - 40;
  }

  ctx.translate(x, y);
  ctx.rotate(Math.PI / 4);
  ctx.translate(-x, -y);

  ctx.fillStyle = '#92bce1';
  ctx.fillRect(x, y, 7, 7);

  ctx.strokeStyle = '#0275d8';
  ctx.lineWidth = 1;
  ctx.strokeRect(x, y, 7, 7);
};

const stdDeviation = (sum: number, values: number[]): number => {
  const avg = sum / values.length;
  let squaredAvg = 0;
  values.map((value) => (squaredAvg += (value - avg) ** 2));
  squaredAvg = squaredAvg / values.length;
  return Math.sqrt(squaredAvg);
};

const exValue = (exemplar: GraphExemplar): number => exemplar.data[0][1];
