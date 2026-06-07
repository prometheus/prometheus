// Result type for /api/v1/alerts endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#querying-target-metadata
export type MetadataResult = Record<
  string,
  { type: string; help: string; unit: string }[]
>;
