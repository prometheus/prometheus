// Result type for /api/v1/status/self_metrics endpoint.

export interface SelfMetricQuantile {
  quantile: string;
  value: string;
}

export interface SelfMetricBucket {
  upperBound: string;
  cumulativeCount: string;
}

export interface SelfMetric {
  labels?: Record<string, string>;
  value?: string;
  quantiles?: SelfMetricQuantile[];
  buckets?: SelfMetricBucket[];
}

export interface SelfMetricFamily {
  name: string;
  help: string;
  type: string;
  unit?: string;
  metrics: SelfMetric[];
}

export type SelfMetricsResult = SelfMetricFamily[];
