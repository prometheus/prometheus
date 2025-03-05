// Result type for /api/v1/status/config endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#config
export default interface ConfigResult {
  yaml: string;
}
