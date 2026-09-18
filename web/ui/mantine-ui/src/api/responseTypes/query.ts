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

// The estimated or actual resource cost of a query. Estimates may be higher
// or lower than the actual cost.
export type CostEstimate = {
  seriesTouched: number;
  samplesRead: number;
  // Only reported for the actual cost of a query, and omitted when zero.
  peakSamples?: number;
};

// The cost estimated before execution paired with the cost measured during
// execution. Only returned when the "cost" query parameter is set and the
// query-cost feature is enabled on the server.
export type QueryCostComparison = {
  estimated: CostEstimate;
  actual: CostEstimate;
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
) & { stats?: QueryStats; cost?: QueryCostComparison };

// Result type for /api/v1/query_range endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#range-queries
export type RangeQueryResult = {
  resultType: "matrix";
  result: RangeSamples[];
  stats?: QueryStats;
  cost?: QueryCostComparison;
};

// Result type for the /api/v1/query_cost and /api/v1/query_range_cost
// endpoints, which estimate a query's cost without executing it.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#query-cost
export type QueryCostResult = {
  estimate: CostEstimate;
};
