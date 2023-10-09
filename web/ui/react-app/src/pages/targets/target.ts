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

export interface ScrapePool {
  upCount: number;
  targets: Target[];
}

export interface ScrapePools {
  [scrapePool: string]: ScrapePool;
}

export const groupTargets = (targets: Target[]): ScrapePools =>
  targets.reduce((pools: ScrapePools, target: Target) => {
    const { health, scrapePool } = target;
    const up = health.toLowerCase() === 'up' ? 1 : 0;
    if (!pools[scrapePool]) {
      pools[scrapePool] = {
        upCount: 0,
        targets: [],
      };
    }
    pools[scrapePool].targets.push(target);
    pools[scrapePool].upCount += up;
    return pools;
  }, {});

export const getColor = (health: string): string => {
  switch (health.toLowerCase()) {
    case 'up':
      return 'success';
    case 'down':
      return 'danger';
    default:
      return 'warning';
  }
};

export interface TargetHealthFilters {
  healthy: boolean;
  unhealthy: boolean;
  unknown: boolean;
}

export const filterTargetsByHealth = (health: string, filters: TargetHealthFilters): boolean => {
  switch (health.toLowerCase()) {
    case 'up':
      return filters.healthy;
    case 'down':
      return filters.unhealthy;
    default:
      return filters.unknown;
  }
};
