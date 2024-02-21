type RuleState = "pending" | "firing" | "inactive";

export interface Alert {
  labels: Record<string, string>;
  state: RuleState;
  value: string;
  annotations: Record<string, string>;
  activeAt: string;
  keepFiringSince: string;
}

type CommonRuleFields = {
  name: string;
  query: string;
  evaluationTime: string;
  health: string;
  lastError?: string;
  lastEvaluation: string;
};

type AlertingRule = {
  type: "alerting";
  // For alerting rules, the 'labels' field is always present, even when there are no labels.
  labels: Record<string, string>;
  annotations: Record<string, string>;
  duration: number;
  keepFiringFor: number;
  state: RuleState;
  alerts: Alert[];
} & CommonRuleFields;

type RecordingRule = {
  type: "recording";
  // For recording rules, the 'labels' field is only present when there are labels.
  labels?: Record<string, string>;
} & CommonRuleFields;

export type Rule = AlertingRule | RecordingRule;

interface RuleGroup {
  name: string;
  file: string;
  interval: string;
  rules: Rule[];
  evaluationTime: string;
  lastEvaluation: string;
}

export interface RulesMap {
  groups: RuleGroup[];
}
