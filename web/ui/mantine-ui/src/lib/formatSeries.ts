import {
  maybeQuoteLabelName,
  metricContainsExtendedCharset,
} from "../promql/utils";
import { escapeString } from "./escapeString";

// TODO: Maybe replace this with the new PromLens-derived serialization code in src/promql/serialize.ts?
export const formatSeries = (labels: { [key: string]: string }): string => {
  if (labels === null) {
    return "scalar";
  }

  if (metricContainsExtendedCharset(labels.__name__ || "")) {
    return `{"${escapeString(labels.__name__)}",${Object.entries(labels)
      .filter(([k]) => k !== "__name__")
      .map(([k, v]) => `${maybeQuoteLabelName(k)}="${escapeString(v)}"`)
      .join(", ")}}`;
  }

  return `${labels.__name__ || ""}{${Object.entries(labels)
    .filter(([k]) => k !== "__name__")
    .map(([k, v]) => `${maybeQuoteLabelName(k)}="${escapeString(v)}"`)
    .join(", ")}}`;
};
