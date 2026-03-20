import { InstantSample, Metric } from "../api/responseTypes/query";
import {
  formatPrometheusFloat,
  parsePrometheusFloat,
} from "../lib/formatFloatValue";
import {
  binaryOperatorType,
  vectorMatchCardinality,
  VectorMatching,
} from "./ast";
import { isComparisonOperator, isSetOperator } from "./utils";

// We use a special (otherwise invalid) sample value to indicate that
// a sample has been filtered away by a comparison operator.
export const filteredSampleValue = "filtered";

export enum MatchErrorType {
  multipleMatchesForOneToOneMatching = "multipleMatchesForOneToOneMatching",
  multipleMatchesOnBothSides = "multipleMatchesOnBothSides",
  multipleMatchesOnOneSide = "multipleMatchesOnOneSide",
}

// There's no group_x() modifier, but one of the sides has multiple matches.
export interface MultipleMatchesForOneToOneMatchingError {
  type: MatchErrorType.multipleMatchesForOneToOneMatching;
  dupeSide: "left" | "right";
}

// There's no group_x() modifier and there are multiple matches on both sides.
// This is good to keep as a separate error from MultipleMatchesForOneToOneMatchingError
// because it can't be fixed by adding group_x() but rather by expanding the set of
// matching labels.
export interface MultipleMatchesOnBothSidesError {
  type: MatchErrorType.multipleMatchesOnBothSides;
}

// There's a group_x() modifier, but the "one" side has multiple matches. This could mean
// that either the matching labels are not sufficient or that group_x() is the wrong way around.
export interface MultipleMatchesOnOneSideError {
  type: MatchErrorType.multipleMatchesOnOneSide;
}

export type VectorMatchError =
  | MultipleMatchesForOneToOneMatchingError
  | MultipleMatchesOnBothSidesError
  | MultipleMatchesOnOneSideError;

export type MaybeFilledInstantSample = InstantSample & {
  // If the sample was filled in via a fill(...) modifier, this is true.
  filled?: boolean;
};

// A single match group as produced by a vector-to-vector binary operation, with all of its
// left-hand side and right-hand side series, as well as a result and error, if applicable.
export type BinOpMatchGroup = {
  groupLabels: Metric;
  rhs: MaybeFilledInstantSample[];
  rhsCount: number; // Number of samples before applying limits.
  lhs: MaybeFilledInstantSample[];
  lhsCount: number; // Number of samples before applying limits.
  result: {
    sample: InstantSample;
    // Which "many"-side sample did this sample come from? This is needed for use cases where
    // we want to style the corresponding "many" side input sample and the result sample in
    // a similar way (e.g. shading them in the same color) to be able to trace which "many"
    // side sample a result sample came from.
    manySideIdx: number;
  }[];
  error: VectorMatchError | null;
};

// The result of computeVectorVectorBinOp(), modeling the match groups produced by a
// vector-to-vector binary operation.
export type BinOpMatchGroups = {
  [sig: string]: BinOpMatchGroup;
};

export type BinOpResult = {
  groups: BinOpMatchGroups;
  // Can differ from the number of returned groups if a limit was applied.
  numGroups: number;
};

// FNV-1a hash parameters.
const FNV_PRIME = 0x01000193;
const OFFSET_BASIS = 0x811c9dc5;
const SEP = "\uD800".charCodeAt(0); // Using a Unicode "high surrogate" code point as a separator. These should not appear by themselves (without a low surrogate pairing) in a valid Unicode string.

// Compute an FNV-1a hash over a given set of values in order to
// produce a signature for a match group.
export const fnv1a = (values: string[]): string => {
  let h = OFFSET_BASIS;
  for (let i = 0; i < values.length; i++) {
    // Skip labels that are not set on the metric.
    if (values[i] !== undefined) {
      for (let c = 0; c < values[i].length; c++) {
        h ^= values[i].charCodeAt(c);
        h *= FNV_PRIME;
      }
    }

    if (i < values.length - 1) {
      h ^= SEP;
      h *= FNV_PRIME;
    }
  }
  return h.toString();
};

// Return a function that generates the match group signature for a given label set.
const signatureFunc = (on: boolean, names: string[]) => {
  names.sort();

  if (on) {
    return (lset: Metric): string => {
      return fnv1a(names.map((ln: string) => lset[ln]));
    };
  }

  return (lset: Metric): string =>
    fnv1a(
      Object.keys(lset)
        .filter((ln) => !names.includes(ln) && ln !== "__name__")
        .map((ln) => lset[ln])
    );
};

// For a given metric, return only the labels used for matching.
const matchLabels = (metric: Metric, on: boolean, labels: string[]): Metric => {
  const result: Metric = {};
  for (const name in metric) {
    if (labels.includes(name) === on && (on || name !== "__name__")) {
      result[name] = metric[name];
    }
  }
  return result;
};

export const scalarBinOp = (
  op: binaryOperatorType,
  lhs: number,
  rhs: number
): number => {
  const { value, keep } = vectorElemBinop(op, lhs, rhs);
  if (isComparisonOperator(op)) {
    return Number(keep);
  }

  return value;
};

export const vectorElemBinop = (
  op: binaryOperatorType,
  lhs: number,
  rhs: number
): { value: number; keep: boolean } => {
  switch (op) {
    case binaryOperatorType.add:
      return { value: lhs + rhs, keep: true };
    case binaryOperatorType.sub:
      return { value: lhs - rhs, keep: true };
    case binaryOperatorType.mul:
      return { value: lhs * rhs, keep: true };
    case binaryOperatorType.div:
      return { value: lhs / rhs, keep: true };
    case binaryOperatorType.pow:
      return { value: Math.pow(lhs, rhs), keep: true };
    case binaryOperatorType.mod:
      return { value: lhs % rhs, keep: true };
    case binaryOperatorType.eql:
      return { value: lhs, keep: lhs === rhs };
    case binaryOperatorType.neq:
      return { value: lhs, keep: lhs !== rhs };
    case binaryOperatorType.gtr:
      return { value: lhs, keep: lhs > rhs };
    case binaryOperatorType.lss:
      return { value: lhs, keep: lhs < rhs };
    case binaryOperatorType.gte:
      return { value: lhs, keep: lhs >= rhs };
    case binaryOperatorType.lte:
      return { value: lhs, keep: lhs <= rhs };
    case binaryOperatorType.atan2:
      return { value: Math.atan2(lhs, rhs), keep: true };
    default:
      throw new Error("invalid binop");
  }
};

// Operations that change the metric's original meaning should drop the metric name from the result.
const shouldDropMetricName = (op: binaryOperatorType): boolean =>
  [
    binaryOperatorType.add,
    binaryOperatorType.sub,
    binaryOperatorType.mul,
    binaryOperatorType.div,
    binaryOperatorType.pow,
    binaryOperatorType.mod,
    binaryOperatorType.atan2,
  ].includes(op);

// Compute the time series labels for the result metric.
export const resultMetric = (
  lhs: Metric,
  rhs: Metric,
  op: binaryOperatorType,
  matching: VectorMatching
): Metric => {
  const result: Metric = {};

  // Start out with all labels from the LHS.
  for (const name in lhs) {
    result[name] = lhs[name];
  }

  // Drop metric name for operations that change the metric's meaning.
  if (shouldDropMetricName(op)) {
    delete result.__name__;
  }

  // Keep only match group labels for 1:1 matches.
  if (matching.card === vectorMatchCardinality.oneToOne) {
    if (matching.on) {
      // Drop all labels that are not in the "on" clause.
      for (const name in result) {
        if (!matching.labels.includes(name)) {
          delete result[name];
        }
      }
    } else {
      // Drop all labels that are in the "ignoring" clause.
      for (const name of matching.labels) {
        delete result[name];
      }
    }
  }

  // Include extra labels from the RHS that were mentioned in a group_x(...) modifier.
  matching.include.forEach((name) => {
    if (name in rhs) {
      result[name] = rhs[name];
    } else {
      // If we are trying to include a label from the "one" side that is not actually set there,
      // we need to make sure that we don't accidentally take its value from the "many" side
      // if it exists there.
      //
      // Example to provoke this case:
      //
      // up == on(job, instance) group_left(__name__) node_exporter_build_info*1
      delete result[name];
    }
  });

  return result;
};

// Compute the match groups and results for each match group for a binary operator between two vectors.
// In the error case, the match groups are still populated and returned, but the error field is set for
// the respective group. Results are not populated for error cases, since especially in the case of a
// many-to-many matching, the cross-product output can become prohibitively expensive.
export const computeVectorVectorBinOp = (
  op: binaryOperatorType,
  matching: VectorMatching,
  bool: boolean,
  lhs: InstantSample[],
  rhs: InstantSample[],
  limits?: {
    maxGroups?: number;
    maxSeriesPerGroup?: number;
  }
): BinOpResult => {
  // For the simplification of further calculations, we assume that the "one" side of a one-to-many match
  // is always the right-hand side of the binop and swap otherwise to ensure this. We swap back in the end.
  [lhs, rhs] =
    matching.card === vectorMatchCardinality.oneToMany
      ? [rhs, lhs]
      : [lhs, rhs];

  const groups: BinOpMatchGroups = {};
  const sigf = signatureFunc(matching.on, matching.labels);

  // While we only use this set to compute a count of limited groups in the end, we can encounter each
  // group multiple times (since multiple series can map to the same group). So we need to use a set
  // to track which groups we've already counted.
  const outOfLimitGroups = new Set<string>();

  // Add all RHS samples to the grouping map.
  rhs.forEach((rs) => {
    const sig = sigf(rs.metric);

    if (!(sig in groups)) {
      if (limits?.maxGroups && Object.keys(groups).length >= limits.maxGroups) {
        outOfLimitGroups.add(sig);
        return;
      }

      groups[sig] = {
        groupLabels: matchLabels(rs.metric, matching.on, matching.labels),
        lhs: [],
        lhsCount: 0,
        rhs: [],
        rhsCount: 0,
        result: [],
        error: null,
      };
    }

    if (
      !limits?.maxSeriesPerGroup ||
      groups[sig].rhsCount < limits.maxSeriesPerGroup
    ) {
      groups[sig].rhs.push(rs);
    }
    groups[sig].rhsCount++;
  });

  // Add all LHS samples to the grouping map.
  lhs.forEach((ls) => {
    const sig = sigf(ls.metric);

    if (!(sig in groups)) {
      if (limits?.maxGroups && Object.keys(groups).length >= limits.maxGroups) {
        outOfLimitGroups.add(sig);
        return;
      }

      groups[sig] = {
        groupLabels: matchLabels(ls.metric, matching.on, matching.labels),
        lhs: [],
        lhsCount: 0,
        rhs: [],
        rhsCount: 0,
        result: [],
        error: null,
      };
    }

    if (
      !limits?.maxSeriesPerGroup ||
      groups[sig].lhsCount < limits.maxSeriesPerGroup
    ) {
      groups[sig].lhs.push(ls);
    }
    groups[sig].lhsCount++;
  });

  // Check for any LHS / RHS with no series and fill in default values, if specified.
  Object.values(groups).forEach((mg) => {
    if (mg.lhs.length === 0 && matching.fillValues.lhs !== null) {
      mg.lhs.push({
        metric: mg.groupLabels,
        value: [0, formatPrometheusFloat(matching.fillValues.lhs as number)],
        filled: true,
      });
      mg.lhsCount = 1;
    }
    if (mg.rhs.length === 0 && matching.fillValues.rhs !== null) {
      mg.rhs.push({
        metric: mg.groupLabels,
        value: [0, formatPrometheusFloat(matching.fillValues.rhs as number)],
        filled: true,
      });
      mg.rhsCount = 1;
    }
  });

  // Annotate the match groups with errors (if any) and populate the results.
  Object.values(groups).forEach((mg) => {
    switch (matching.card) {
      case vectorMatchCardinality.oneToOne:
        if (mg.lhs.length > 1 && mg.rhs.length > 1) {
          mg.error = { type: MatchErrorType.multipleMatchesOnBothSides };
        } else if (mg.lhs.length > 1 || mg.rhs.length > 1) {
          mg.error = {
            type: MatchErrorType.multipleMatchesForOneToOneMatching,
            dupeSide: mg.lhs.length > 1 ? "left" : "right",
          };
        }
        break;
      case vectorMatchCardinality.oneToMany:
      case vectorMatchCardinality.manyToOne:
        if (mg.rhs.length > 1) {
          mg.error = {
            type: MatchErrorType.multipleMatchesOnOneSide,
          };
        }
        break;
      case vectorMatchCardinality.manyToMany:
        // Should be a set operator - these don't have errors that aren't caught during parsing.
        if (!isSetOperator(op)) {
          throw new Error(
            "unexpected many-to-many matching for non-set operator"
          );
        }
        break;
      default:
        throw new Error("unknown vector matching cardinality");
    }

    if (mg.error) {
      // We don't populate results for error cases, as especially in the case of a
      // many-to-many matching, the cross-product output can become expensive,
      // and the LHS/RHS are sufficient to diagnose the matching problem.
      return;
    }

    if (isSetOperator(op)) {
      // Add LHS samples to the result, depending on specific operator condition and RHS length.
      mg.lhs.forEach((ls, lIdx) => {
        if (
          (op === binaryOperatorType.and && mg.rhs.length > 0) ||
          (op === binaryOperatorType.unless && mg.rhs.length === 0) ||
          op === binaryOperatorType.or
        ) {
          mg.result.push({
            sample: {
              metric: ls.metric,
              value: ls.value,
            },
            manySideIdx: lIdx,
          });
        }
      });

      // For OR, also add all RHS samples to the result if the LHS for the group is empty.
      if (op === binaryOperatorType.or) {
        mg.rhs.forEach((rs, rIdx) => {
          if (mg.lhs.length === 0) {
            mg.result.push({
              sample: {
                metric: rs.metric,
                value: rs.value,
              },
              manySideIdx: rIdx,
            });
          }
        });
      }
    } else {
      // Calculate the results for this match group.
      mg.rhs.forEach((rs) => {
        mg.lhs.forEach((ls, lIdx) => {
          if (!ls.value || !rs.value) {
            // TODO: Implement native histogram support.
            throw new Error("native histogram support not implemented yet");
          }

          const [vl, vr] =
            matching.card !== vectorMatchCardinality.oneToMany
              ? [ls.value[1], rs.value[1]]
              : [rs.value[1], ls.value[1]];

          let { value, keep } = vectorElemBinop(
            op,
            parsePrometheusFloat(vl),
            parsePrometheusFloat(vr)
          );

          const metric = resultMetric(ls.metric, rs.metric, op, matching);
          if (bool) {
            value = keep ? 1.0 : 0.0;
            delete metric.__name__;
          }

          mg.result.push({
            sample: {
              metric: metric,
              value: [
                ls.value[0],
                keep || bool
                  ? formatPrometheusFloat(value)
                  : filteredSampleValue,
              ],
            },
            manySideIdx: lIdx,
          });
        });
      });
    }
  });

  // If we originally swapped the LHS and RHS, swap them back to the original order.
  if (matching.card === vectorMatchCardinality.oneToMany) {
    Object.keys(groups).forEach((sig) => {
      [groups[sig].lhs, groups[sig].rhs] = [groups[sig].rhs, groups[sig].lhs];
      [groups[sig].lhsCount, groups[sig].rhsCount] = [
        groups[sig].rhsCount,
        groups[sig].lhsCount,
      ];
    });
  }

  return {
    groups,
    numGroups: Object.keys(groups).length + outOfLimitGroups.size,
  };
};
