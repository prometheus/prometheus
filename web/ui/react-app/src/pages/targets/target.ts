export interface Labels {
  [key: string]: string;
}

export interface Target {
  discoveredLabels: Labels;
  labels: Labels;
  scrapeJob: string;
  scrapeUrl: string;
  lastError: string;
  lastScrape: string;
  lastScrapeDuration: number;
  health: string;
}

export interface TargetGroupMetadata {
  up: number;
}

export interface TargetGroup {
  metadata: TargetGroupMetadata;
  targets: Target[];
}

export interface TargetGroups {
  [scrapeJob: string]: TargetGroup;
}

export const groupTargets = (targets: Target[]): TargetGroups =>
  targets.reduce((groups: TargetGroups, target: Target) => {
    const { health, scrapeJob } = target;
    const up = health.toLowerCase() === 'up' ? 1 : 0;
    if (!groups[scrapeJob]) {
      groups[scrapeJob] = {
        metadata: { up: 0 },
        targets: [],
      };
    }
    groups[scrapeJob].targets.push(target);
    groups[scrapeJob].metadata.up += up;
    return groups;
  }, {});
