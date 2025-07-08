export interface Labels {
  [key: string]: string;
}

export type Target = {
  discoveredLabels: Labels;
  labels: Labels;
  scrapePool: string;
  scrapeUrl: string;
  globalUrl: string;
  lastError: string;
  lastScrape: string;
  lastScrapeDuration: number;
  health: string;
  scrapeInterval: string;
  scrapeTimeout: string;
};

export interface DroppedTarget {
  discoveredLabels: Labels;
  scrapePool: string;
}

// Result type for /api/v1/targets endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#targets
export type TargetsResult = {
  activeTargets: Target[];
  droppedTargets: DroppedTarget[];
  droppedTargetCounts: Record<string, number>;
};
