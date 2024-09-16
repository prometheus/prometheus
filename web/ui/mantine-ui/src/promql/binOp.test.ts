import { describe, expect, it } from "vitest";
import {
  BinOpResult,
  MatchErrorType,
  computeVectorVectorBinOp,
  filteredSampleValue,
  fnv1a,
  resultMetric,
} from "./binOp";
import {
  VectorMatching,
  binaryOperatorType,
  vectorMatchCardinality,
} from "./ast";
import { InstantSample, Metric } from "../api/responseTypes/query";

type TestCase = {
  desc: string;
  op: binaryOperatorType;
  matching: VectorMatching;
  bool?: boolean;
  lhs: InstantSample[];
  rhs: InstantSample[];
  limits?: {
    maxGroups?: number;
    maxSeriesPerGroup?: number;
  };
  result: BinOpResult;
};

const testMetricA: InstantSample[] = [
  {
    metric: { __name__: "metric_a", label1: "a", label2: "x", same: "same" },
    value: [0, "1"],
  },
  {
    metric: { __name__: "metric_a", label1: "a", label2: "y", same: "same" },
    value: [0, "2"],
  },
  {
    metric: { __name__: "metric_a", label1: "b", label2: "x", same: "same" },
    value: [0, "3"],
  },
  {
    metric: { __name__: "metric_a", label1: "b", label2: "y", same: "same" },
    value: [0, "4"],
  },
];

const testMetricB: InstantSample[] = [
  {
    metric: { __name__: "metric_b", label1: "a", label2: "x", same: "same" },
    value: [0, "10"],
  },
  {
    metric: { __name__: "metric_b", label1: "a", label2: "y", same: "same" },
    value: [0, "20"],
  },
  {
    metric: { __name__: "metric_b", label1: "b", label2: "x", same: "same" },
    value: [0, "30"],
  },
  {
    metric: { __name__: "metric_b", label1: "b", label2: "y", same: "same" },
    value: [0, "40"],
  },
];

const testMetricC: InstantSample[] = [
  { metric: { __name__: "metric_c", label1: "a" }, value: [0, "100"] },
  { metric: { __name__: "metric_c", label1: "b" }, value: [0, "200"] },
];

const testCases: TestCase[] = [
  {
    // metric_a - metric_b
    desc: "one-to-one matching on all labels",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "-9"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "-18"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "-27"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "y", same: "same" },
                value: [0, "-36"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a - on(label1, label2) metric_b
    desc: "one-to-one matching on explicit labels",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: true,
      include: [],
      labels: ["label1", "label2"],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x"])]: {
          groupLabels: { label1: "a", label2: "x" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x" },
                value: [0, "-9"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y"])]: {
          groupLabels: { label1: "a", label2: "y" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y" },
                value: [0, "-18"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x"])]: {
          groupLabels: { label1: "b", label2: "x" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x" },
                value: [0, "-27"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y"])]: {
          groupLabels: { label1: "b", label2: "y" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "y" },
                value: [0, "-36"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a - ignoring(same) metric_b
    desc: "one-to-one matching ignoring explicit labels",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: ["same"],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x"])]: {
          groupLabels: { label1: "a", label2: "x" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x" },
                value: [0, "-9"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y"])]: {
          groupLabels: { label1: "a", label2: "y" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y" },
                value: [0, "-18"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x"])]: {
          groupLabels: { label1: "b", label2: "x" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x" },
                value: [0, "-27"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y"])]: {
          groupLabels: { label1: "b", label2: "y" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "y" },
                value: [0, "-36"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_b - metric_c
    desc: "many-to-one matching with no matching labels specified (empty output)",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricB,
    rhs: testMetricC,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          lhsCount: 1,
          rhs: [],
          rhsCount: 0,
          result: [],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          lhsCount: 1,
          rhs: [],
          rhsCount: 0,
          result: [],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          lhsCount: 1,
          rhs: [],
          rhsCount: 0,
          result: [],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          lhsCount: 1,
          rhs: [],
          rhsCount: 0,
          result: [],
          error: null,
        },
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [],
          lhsCount: 0,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          rhsCount: 1,
          result: [],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [],
          lhsCount: 0,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          rhsCount: 1,
          result: [],
          error: null,
        },
      },
      numGroups: 6,
    },
  },
  {
    // metric_b - on(label1) metric_c
    desc: "many-to-one matching with matching labels specified, but no group_left (error)",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricB,
    rhs: testMetricC,
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          rhsCount: 1,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesForOneToOneMatching,
            dupeSide: "left",
          },
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          rhsCount: 1,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesForOneToOneMatching,
            dupeSide: "left",
          },
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_b - on(label1) group_left metric_c
    desc: "many-to-one matching with matching labels specified and group_left",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.manyToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricB,
    rhs: testMetricC,
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "-90"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "-80"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "-170"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: { label1: "b", label2: "y", same: "same" },
                value: [0, "-160"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_c - on(label1) group_right metric_b
    desc: "one-to-many matching with matching labels specified and group_right",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToMany,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricC,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 2,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "90"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "80"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "170"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: { label1: "b", label2: "y", same: "same" },
                value: [0, "160"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_c - on(label1) group_left metric_b
    desc: "one-to-many matching with matching labels specified but incorrect group_left (error)",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.manyToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricC,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 2,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesOnOneSide,
          },
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesOnOneSide,
          },
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a - on(label1) metric_b
    desc: "insufficient matching labels leading to many-to-many matching for intended one-to-one match (error)",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 2,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesOnBothSides,
          },
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [],
          error: {
            type: MatchErrorType.multipleMatchesOnBothSides,
          },
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a < metric_b
    desc: "filter op keeping all series",
    op: binaryOperatorType.lss,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, "2"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, "3"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "y",
                  same: "same",
                },
                value: [0, "4"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a >= metric_b
    desc: "filter op dropping all series",
    op: binaryOperatorType.gte,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, filteredSampleValue],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, filteredSampleValue],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, filteredSampleValue],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "y",
                  same: "same",
                },
                value: [0, filteredSampleValue],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a >= bool metric_b
    desc: "filter op dropping all series, but with bool",
    op: binaryOperatorType.gte,
    bool: true,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "0"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "0"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "0"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "y", same: "same" },
                value: [0, "0"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a < bool metric_b
    desc: "filter op keeping all series, but with bool",
    op: binaryOperatorType.lss,
    bool: true,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "y", same: "same" },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a - metric_b
    desc: "exceeding the match group limit",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.oneToOne,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    limits: { maxGroups: 2 },
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "-9"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "y", same: "same" },
                value: [0, "-18"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_c - on(label1) group_left metric_b
    desc: "exceeding the per-group series limit",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.manyToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricB,
    rhs: testMetricC,
    limits: { maxSeriesPerGroup: 1 },
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "-90"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "b" },
              value: [0, "200"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "b", label2: "x", same: "same" },
                value: [0, "-170"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_c - on(label1) group_left metric_b
    desc: "exceeding both group limit and per-group series limit",
    op: binaryOperatorType.sub,
    matching: {
      card: vectorMatchCardinality.manyToOne,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricB,
    rhs: testMetricC,
    limits: { maxGroups: 1, maxSeriesPerGroup: 1 },
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: { __name__: "metric_c", label1: "a" },
              value: [0, "100"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: { label1: "a", label2: "x", same: "same" },
                value: [0, "-90"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a and metric b
    desc: "and operator with no matching labels and matching groups",
    op: binaryOperatorType.and,
    matching: {
      card: vectorMatchCardinality.manyToMany,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA,
    rhs: testMetricB,
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: { label1: "a", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "10"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: { label1: "a", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, "2"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: { label1: "b", label2: "x", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, "3"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: { label1: "b", label2: "y", same: "same" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "4"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "y",
                  same: "same",
                },
                value: [0, "4"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
  {
    // metric_a[0...2] and on(label1) metric_b[1...3]
    desc: "and operator with matching label and series on each side",
    op: binaryOperatorType.and,
    matching: {
      card: vectorMatchCardinality.manyToMany,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricA.slice(0, 3),
    rhs: testMetricB.slice(1, 4),
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, "2"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, "3"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a[0...2] unless on(label1) metric_b[1...3]
    desc: "unless operator with matching label and series on each side",
    op: binaryOperatorType.unless,
    matching: {
      card: vectorMatchCardinality.manyToMany,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricA.slice(0, 3),
    rhs: testMetricB.slice(1, 4),
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a[0...2] or on(label1) metric_b[1...3]
    desc: "or operator with matching label and series on each side",
    op: binaryOperatorType.or,
    matching: {
      card: vectorMatchCardinality.manyToMany,
      on: true,
      include: [],
      labels: ["label1"],
    },
    lhs: testMetricA.slice(0, 3),
    rhs: testMetricB.slice(1, 4),
    result: {
      groups: {
        [fnv1a(["a"])]: {
          groupLabels: { label1: "a" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 2,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, "2"],
              },
              manySideIdx: 1,
            },
          ],
          error: null,
        },
        [fnv1a(["b"])]: {
          groupLabels: { label1: "b" },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 2,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, "3"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 2,
    },
  },
  {
    // metric_a[0...2] or metric_b[1...3]
    desc: "or operator with only partial overlap",
    op: binaryOperatorType.or,
    matching: {
      card: vectorMatchCardinality.manyToMany,
      on: false,
      include: [],
      labels: [],
    },
    lhs: testMetricA.slice(0, 3),
    rhs: testMetricB.slice(1, 4),
    result: {
      groups: {
        [fnv1a(["a", "x", "same"])]: {
          groupLabels: {
            label1: "a",
            label2: "x",
            same: "same",
          },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "x",
                same: "same",
              },
              value: [0, "1"],
            },
          ],
          lhsCount: 1,
          rhs: [],
          rhsCount: 0,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "x",
                  same: "same",
                },
                value: [0, "1"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["a", "y", "same"])]: {
          groupLabels: {
            label1: "a",
            label2: "y",
            same: "same",
          },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "2"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "a",
                label2: "y",
                same: "same",
              },
              value: [0, "20"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "a",
                  label2: "y",
                  same: "same",
                },
                value: [0, "2"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "x", "same"])]: {
          groupLabels: {
            label1: "b",
            label2: "x",
            same: "same",
          },
          lhs: [
            {
              metric: {
                __name__: "metric_a",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "3"],
            },
          ],
          lhsCount: 1,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "x",
                same: "same",
              },
              value: [0, "30"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_a",
                  label1: "b",
                  label2: "x",
                  same: "same",
                },
                value: [0, "3"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
        [fnv1a(["b", "y", "same"])]: {
          groupLabels: {
            label1: "b",
            label2: "y",
            same: "same",
          },
          lhs: [],
          lhsCount: 0,
          rhs: [
            {
              metric: {
                __name__: "metric_b",
                label1: "b",
                label2: "y",
                same: "same",
              },
              value: [0, "40"],
            },
          ],
          rhsCount: 1,
          result: [
            {
              sample: {
                metric: {
                  __name__: "metric_b",
                  label1: "b",
                  label2: "y",
                  same: "same",
                },
                value: [0, "40"],
              },
              manySideIdx: 0,
            },
          ],
          error: null,
        },
      },
      numGroups: 4,
    },
  },
];

describe("binOp", () => {
  describe("resultMetric", () => {
    it("should drop metric name for operations that change meaning", () => {
      const lhs: Metric = { __name__: "metric_a", label1: "value1" };
      const rhs: Metric = { __name__: "metric_b", label2: "value2" };
      const op = binaryOperatorType.add;
      const matching = {
        card: vectorMatchCardinality.oneToMany,
        on: true,
        labels: ["label1"],
        include: [],
      };

      const result = resultMetric(lhs, rhs, op, matching);

      expect(result.__name__).toBeUndefined();
      expect(result["label1"]).toEqual("value1");
    });

    it("should keep only on labels for 1:1 matching", () => {
      const lhs: Metric = {
        __name__: "metric_a",
        label1: "value1",
        label2: "value2",
      };
      const rhs: Metric = {
        __name__: "metric_b",
        label1: "value1",
        label3: "value3",
      };
      const op = binaryOperatorType.add;
      const matching = {
        card: vectorMatchCardinality.oneToOne,
        on: true,
        labels: ["label1"],
        include: [],
      };

      const result = resultMetric(lhs, rhs, op, matching);

      expect(result).toEqual({ label1: "value1" });
    });

    it("should include extra labels from RHS mentioned in group_x", () => {
      const lhs: Metric = { __name__: "metric_a", label1: "value1" };
      const rhs: Metric = {
        __name__: "metric_b",
        label1: "value1",
        label2: "value2",
      };
      const op = binaryOperatorType.add;
      const matching = {
        card: vectorMatchCardinality.oneToMany,
        on: true,
        labels: ["label1"],
        include: ["label2"],
      };

      const result = resultMetric(lhs, rhs, op, matching);

      expect(result).toEqual({
        label1: "value1",
        label2: "value2",
      });
    });
  });

  describe("computeVectorVectorBinOp", () => {
    testCases.forEach((tc) => {
      it(tc.desc, () => {
        expect(
          computeVectorVectorBinOp(
            tc.op,
            tc.matching,
            !!tc.bool,
            tc.lhs,
            tc.rhs,
            tc.limits
          )
        ).toEqual(tc.result);
      });
    });
  });
});
