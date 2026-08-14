type RuleState = "pending" | "firing" | "inactive" | "unknown";

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
  health: "ok" | "unknown" | "err";
  lastError?: string;
  lastEvaluation: string;
};

export type AlertingRule = {
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

export interface RuleGroup {
  name: string;
  file: string;
  interval: string;
  rules: Rule[];
  evaluationTime: string;
  lastEvaluation: string;
}

export type AlertingRuleGroup = Omit<RuleGroup, "rules"> & {
  rules: AlertingRule[];
};

// Result type for /api/v1/alerts endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#alerts
export interface RulesResult {
  groups: RuleGroup[];
}

// Same as RulesResult above, but can be used when the caller ensures via a
// "type=alert" query parameter that all rules are alerting rules.
export interface AlertingRulesResult {
  groups: AlertingRuleGroup[];
}
