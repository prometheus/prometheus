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

export interface TSDBMap {
  headStats: HeadStats;
  seriesCountByMetricName: Stats[];
  labelValueCountByLabelName: Stats[];
  memoryInBytesByLabelName: Stats[];
  seriesCountByLabelValuePair: Stats[];
}
