import { Labels } from "./targets";

export type RelabelStep = {
  rule: { [key: string]: unknown };
  output: Labels;
  keep: boolean;
};

// Result type for /api/v1/relabel_steps endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#relabel_steps
export type RelabelStepsResult = {
  steps: RelabelStep[];
};
