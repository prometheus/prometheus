import { Alert, RuleState } from '../pages/alerts/AlertContents';

export interface Metric {
  [key: string]: string;
}

export interface QueryParams {
  startTime: number;
  endTime: number;
  resolution: number;
}

export interface Rule {
  alerts: Alert[];
  annotations: Record<string, string>;
  duration: number;
  evaluationTime: string;
  health: string;
  labels: Record<string, string>;
  lastError?: string;
  lastEvaluation: string;
  name: string;
  query: string;
  state: RuleState;
  type: string;
}
