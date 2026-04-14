// Result type for /api/v1/series endpoint.

import { Metric } from "./query";

// See: https://prometheus.io/docs/prometheus/latest/querying/api/#finding-series-by-label-matchers
export type SeriesResult = Metric[];
