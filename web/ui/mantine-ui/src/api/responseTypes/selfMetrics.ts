// Result type for /api/v1/status/self_metrics endpoint.
// The response uses the standard ProtoJSON format for io.prometheus.client.MetricFamily.
// See https://protobuf.dev/programming-guides/json/

export interface ProtoLabelPair {
  name: string;
  value: string;
}

export interface ProtoGauge {
  value: number;
}

export interface ProtoCounter {
  value: number;
  exemplar?: ProtoExemplar;
  createdTimestamp?: string;
}

export interface ProtoQuantile {
  quantile: number;
  value: number;
}

export interface ProtoSummary {
  sampleCount: string;
  sampleSum: number;
  quantile?: ProtoQuantile[];
  createdTimestamp?: string;
}

export interface ProtoBucket {
  cumulativeCount: string;
  upperBound: number;
  exemplar?: ProtoExemplar;
}

export interface ProtoHistogram {
  sampleCount: string;
  sampleSum: number;
  bucket?: ProtoBucket[];
  createdTimestamp?: string;
}

export interface ProtoExemplar {
  label?: ProtoLabelPair[];
  value: number;
  timestamp?: string;
}

export interface ProtoUntyped {
  value: number;
}

export interface ProtoMetric {
  label?: ProtoLabelPair[];
  gauge?: ProtoGauge;
  counter?: ProtoCounter;
  summary?: ProtoSummary;
  histogram?: ProtoHistogram;
  untyped?: ProtoUntyped;
  timestampMs?: string;
}

export interface ProtoMetricFamily {
  name: string;
  help: string;
  type: string;
  metric: ProtoMetric[];
  unit?: string;
}

export type SelfMetricsResult = ProtoMetricFamily[];
