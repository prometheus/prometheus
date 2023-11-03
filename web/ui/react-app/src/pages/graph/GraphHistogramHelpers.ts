/**
 * Inspired by a similar feature in VictoriaMetrics.
 * See https://github.com/VictoriaMetrics/VictoriaMetrics/issues/3384 for more details.
 * Developed by VictoriaMetrics team.
 */

import { GraphExemplar, GraphProps, GraphSeries } from './Graph';

export function isHistogramData(data: GraphProps['data']) {
  if (!data?.result?.length) return false;
  const result = data.result;
  if (result.length < 2) return false;
  const histogramLabels = ['le'];

  const firstLabels = Object.keys(result[0].metric).filter((n) => !histogramLabels.includes(n));
  const isHistogram = result.every((r) => {
    const labels = Object.keys(r.metric).filter((n) => !histogramLabels.includes(n));
    return firstLabels.length === labels.length && labels.every((l) => r.metric[l] === result[0].metric[l]);
  });

  return isHistogram && result.every((r) => histogramLabels.some((l) => l in r.metric));
}

export function prepareHistogramData(buckets: GraphSeries[]) {
  if (!buckets.every((a) => a.labels.le)) return buckets;

  const sortedBuckets = buckets.sort((a, b) => promValueToNumber(a.labels.le) - promValueToNumber(b.labels.le));
  let prevBucket: GraphSeries | GraphExemplar = { data: [[0, 0]], labels: {}, color: '', index: 0 };
  const result: GraphSeries[] = [];
  let index = 0;

  for (const bucket of sortedBuckets) {
    const values = [];
    const { data, labels, color } = bucket;

    for (const [timestamp, value] of data) {
      const prevVal = prevBucket.data.find((v) => v[0] === timestamp)?.[1] || 0;
      const newVal = Number(value) - +prevVal;
      values.push([Number(timestamp), newVal]);
    }

    result.push({
      data: values,
      labels,
      color,
      index,
    });
    prevBucket = bucket;
    index += 1;
  }
  return result;
}

export function promValueToNumber(s: string) {
  switch (s) {
    case 'NaN':
      return NaN;
    case 'Inf':
    case '+Inf':
      return Infinity;
    case '-Inf':
      return -Infinity;
    default:
      return parseFloat(s);
  }
}
