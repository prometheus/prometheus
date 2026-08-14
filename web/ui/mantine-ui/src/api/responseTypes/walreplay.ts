// Result type for /api/v1/status/walreplay endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#wal-replay-stats
export interface WALReplayStatus {
  min: number;
  max: number;
  current: number;
}
