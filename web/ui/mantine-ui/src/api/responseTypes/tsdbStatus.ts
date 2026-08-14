interface Stats {
  name: string;
  value: number;
}

interface HeadStats {
  numSeries: number;
  numLabelPairs: number;
  chunkCount: number;
  minTime: number;
  maxTime: number;
}

// Result type for /api/v1/status/tsdb endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#tsdb-stats
export interface TSDBStatusResult {
  headStats: HeadStats;
  seriesCountByMetricName: Stats[];
  labelValueCountByLabelName: Stats[];
  memoryInBytesByLabelName: Stats[];
  seriesCountByLabelValuePair: Stats[];
}
