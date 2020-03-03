import { Alert, RuleState } from '../pages/alerts/AlertContents';

export interface Metric {
  [key: string]: string;
}

export interface QueryParams {
  startTime: number;
  endTime: number;
  resolution: number;
}

export interface BaseRule {
  name: string;
  query: string;
  labels: Record<string, string>;
  health: 'ok' | 'err' | 'unknown';
  lastError?: string;
  evaluationTime: string;
  lastEvaluation: string;
}

export interface AlertingRule extends BaseRule {
  alerts: Alert[];
  duration: number;
  state: RuleState;
  annotations: Record<string, string>;
  type: 'alerting';
}

export interface RecordingRule extends BaseRule {
  type: 'recording';
}

export type Rule = RecordingRule | AlertingRule;
