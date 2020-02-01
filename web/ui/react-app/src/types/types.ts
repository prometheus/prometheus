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
  type: 'alerting' | 'recording';
}

export interface AlertingRule extends BaseRule {
  alerts: Alert[];
  duration: number;
  state: RuleState;
  annotations: Record<string, string>;
}

export type RecordingRule = BaseRule;
export type Rule = RecordingRule | AlertingRule;
