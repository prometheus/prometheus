import { Histogram } from "../../types/types";
import {
  calculateDefaultExpBucketWidth,
  findZeroAxisLeft,
} from "./HistogramHelpers";

describe("calculateDefaultExpBucketWidth", () => {
  it.each<{
    buckets: NonNullable<Histogram["buckets"]>;
    expected: number;
  }>([
    { buckets: [[3, "-0", "0", "5"]], expected: 1 },
    { buckets: [[3, "-0.001", "0.001", "5"]], expected: 1 },
    { buckets: [[0, "1", "2", "5"]], expected: Math.log(2) },
    { buckets: [[1, "-2", "-1", "5"]], expected: Math.log(2) },
    {
      buckets: [
        [1, "-2", "-1", "5"],
        [3, "-0", "0", "5"],
      ],
      expected: Math.log(2),
    },
  ])("calculates the width for $buckets", ({ buckets, expected }) => {
    expect(
      calculateDefaultExpBucketWidth(buckets[buckets.length - 1], buckets),
    ).toBeCloseTo(expected);
  });
});

describe("findZeroAxisLeft", () => {
  describe("linear scale", () => {
    const cases: {
      name: string;
      rangeMin: number;
      rangeMax: number;
      expected: string;
    }[] = [
      {
        name: "centers the zero axis for a zero-width range",
        rangeMin: -0,
        rangeMax: 0,
        expected: "50%",
      },
      {
        name: "positions the zero axis proportionally when zero is in range",
        rangeMin: -10,
        rangeMax: 30,
        expected: "25%",
      },
      {
        name: "clamps to the left edge for all-positive buckets",
        rangeMin: 0.001,
        rangeMax: 1024,
        expected: "0%",
      },
      {
        name: "clamps to the right edge for all-negative buckets",
        rangeMin: -1024,
        rangeMax: -0.001,
        expected: "100%",
      },
      {
        name: "keeps the left edge when the range starts at zero",
        rangeMin: 0,
        rangeMax: 1024,
        expected: "0%",
      },
      {
        name: "keeps the right edge when the range ends at zero",
        rangeMin: -1024,
        rangeMax: 0,
        expected: "100%",
      },
    ];

    for (const c of cases) {
      test(c.name, () => {
        expect(
          findZeroAxisLeft(
            "linear",
            c.rangeMin,
            c.rangeMax,
            // The remaining arguments are only used by the exponential scale.
            0,
            0,
            -1,
            0,
            0,
            0,
          ),
        ).toBe(c.expected);
      });
    }
  });

  describe("exponential scale", () => {
    test("centers the zero axis for a zero-width range", () => {
      expect(findZeroAxisLeft("exponential", -0, 0, 0, 0, 0, 0, 1, 1)).toBe(
        "50%",
      );
    });

    test("clamps to the left edge for all-positive buckets", () => {
      expect(
        findZeroAxisLeft("exponential", 0.001, 1024, 0.001, 0, -1, 0, 10, 1),
      ).toBe("0%");
    });

    test("clamps to the right edge for all-negative buckets", () => {
      expect(
        findZeroAxisLeft(
          "exponential",
          -1024,
          -0.001,
          0,
          -0.001,
          -1,
          10,
          10,
          1,
        ),
      ).toBe("100%");
    });

    test("positions the zero axis between the buckets around zero", () => {
      expect(
        findZeroAxisLeft(
          "exponential",
          -1024,
          1024,
          0.001,
          -0.001,
          -1,
          5,
          20,
          1,
        ),
      ).toBe("25%");
    });
  });
});
