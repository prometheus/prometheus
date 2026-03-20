import { Alert, RuleState } from '../pages/alerts/AlertContents';

export interface Metric {
  [key: string]: string;
}

export interface Histogram {
  count: string;
  sum: string;
  buckets?: [number, string, string, string][];
}

export interface Exemplar {
  labels: { [key: string]: string };
  value: string;
  timestamp: number;
}

export interface QueryParams {
  startTime: number;
  endTime: number;
  resolution: number;
}

export type Rule = {
  alerts: Alert[];
  annotations: Record<string, string>;
  duration: number;
  keepFiringFor: number;
  evaluationTime: string;
  health: string;
  labels: Record<string, string>;
  lastError?: string;
  lastEvaluation: string;
  name: string;
  query: string;
  state: RuleState;
  type: string;
};

export interface WALReplayData {
  min: number;
  max: number;
  current: number;
}

export interface WALReplayStatus {
  data?: WALReplayData;
}

export type ExemplarData = Array<{ seriesLabels: Metric; exemplars: Exemplar[] }> | undefined;
