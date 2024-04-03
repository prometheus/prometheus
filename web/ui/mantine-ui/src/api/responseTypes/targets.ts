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
}

export type TargetsResult = {
  activeTargets: Target[];
  droppedTargets: DroppedTarget[];
  droppedTargetCounts: Record<string, number>;
};
