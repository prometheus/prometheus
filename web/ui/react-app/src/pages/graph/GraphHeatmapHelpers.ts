import { GraphProps, GraphSeries } from './Graph';

export function isHeatmapData(data: GraphProps['data']) {
  if (!data?.result?.length || data?.result?.length < 2) {
    return false;
  }
  const result = data.result;
  const firstLabels = Object.keys(result[0].metric).filter((label) => label !== 'le');
  return result.every(({ metric }) => {
    const labels = Object.keys(metric).filter((label) => label !== 'le');
    const allLabelsMatch = labels.every((label) => metric[label] === result[0].metric[label]);
    return metric.le && labels.length === firstLabels.length && allLabelsMatch;
  });
}

export function prepareHeatmapData(buckets: GraphSeries[]) {
  if (!buckets.every((a) => a.labels.le)) {
    return buckets;
  }

  const sortedBuckets = buckets.sort((a, b) => promValueToNumber(a.labels.le) - promValueToNumber(b.labels.le));
  const result: GraphSeries[] = [];

  for (let i = 0; i < sortedBuckets.length; i++) {
    const values = [];
    const { data, labels, color } = sortedBuckets[i];

    for (const [timestamp, value] of data) {
      const prevVal = sortedBuckets[i - 1]?.data.find((v) => v[0] === timestamp)?.[1] || 0;
      const newVal = Number(value) - prevVal;
      values.push([Number(timestamp), newVal]);
    }

    result.push({
      data: values,
      labels,
      color,
      index: i,
    });
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
