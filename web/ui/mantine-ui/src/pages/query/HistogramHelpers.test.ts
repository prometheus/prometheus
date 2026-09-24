import { findZeroAxisLeft } from "./HistogramHelpers";

describe("findZeroAxisLeft", () => {
  describe("linear scale", () => {
    const cases: {
      name: string;
      rangeMin: number;
      rangeMax: number;
      expected: string;
    }[] = [
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
