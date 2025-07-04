export interface Metric {
  [key: string]: string;
}

export interface Histogram {
  count: string;
  sum: string;
  buckets?: [number, string, string, string][];
}

export interface InstantSample {
  metric: Metric;
  value?: SampleValue;
  histogram?: SampleHistogram;
}

export interface RangeSamples {
  metric: Metric;
  values?: SampleValue[];
  histograms?: SampleHistogram[];
}

export type SampleValue = [number, string];
export type SampleHistogram = [number, Histogram];

export type QueryStats = {
  timings: Record<string, number>;
  samples: Record<string, number>;
};

// Result type for /api/v1/query endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#instant-queries
export type InstantQueryResult = (
  | {
      resultType: "vector";
      result: InstantSample[];
    }
  | {
      resultType: "matrix";
      result: RangeSamples[];
    }
  | {
      resultType: "scalar";
      result: SampleValue;
    }
  | {
      resultType: "string";
      result: SampleValue;
    }
) & { stats?: QueryStats };

// Result type for /api/v1/query_range endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#range-queries
export type RangeQueryResult = {
  resultType: "matrix";
  result: RangeSamples[];
  stats?: QueryStats;
};
