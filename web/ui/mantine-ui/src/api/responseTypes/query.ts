export interface Metric {
  [key: string]: string;
}

export interface Histogram {
  count: string;
  sum: string;
  buckets?: [number, string, string, string][];
}

// Native-metadata resource attributes for a series, split into identifying
// (stable service identity) and descriptive (everything else). Returned in the
// shared contexts table when the context= query parameter is used.
export interface ResourceAttributes {
  identifying?: Record<string, string>;
  descriptive?: Record<string, string>;
}

export interface SeriesContext {
  resource?: ResourceAttributes;
}

// A change-point in a range series' context: the context id takes effect at
// sample index i (into values) and applies until the next change-point. A null
// id marks samples with no resolvable context.
export interface ContextRun {
  i: number;
  id: string | null;
}

// Reference from a series to the shared contexts table: a bare id when the
// whole series shares one context, or change-points over the sample axis.
export type ContextRef = string | ContextRun[];

export interface InstantSample {
  metric: Metric;
  value?: SampleValue;
  histogram?: SampleHistogram;
  context?: ContextRef;
}

export interface RangeSamples {
  metric: Metric;
  values?: SampleValue[];
  histograms?: SampleHistogram[];
  context?: ContextRef;
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
) & { stats?: QueryStats; contexts?: Record<string, SeriesContext> };

// Result type for /api/v1/query_range endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#range-queries
export type RangeQueryResult = {
  resultType: "matrix";
  result: RangeSamples[];
  stats?: QueryStats;
};
