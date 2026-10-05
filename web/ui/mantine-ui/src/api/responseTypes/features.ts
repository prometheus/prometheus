// Result type for /api/v1/features endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#features
export type FeaturesResult = Record<string, Record<string, boolean>>;
