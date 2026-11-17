import { Histogram } from "../../types/types";
import { bucketRangeString, getHistogramLayout } from "./HistogramHelpers";

type Buckets = NonNullable<Histogram["buckets"]>;

describe.each(["linear", "exponential"])("getHistogramLayout (%s)", (scale) => {
  it.each<{ buckets: Buckets; zero: number }>([
    { buckets: [[3, "-0", "0", "5"]], zero: 50 },
    {
      buckets: [
        [0, "1", "2", "5"],
        [0, "2", "4", "5"],
      ],
      zero: 0,
    },
    {
      buckets: [
        [1, "-4", "-2", "5"],
        [1, "-2", "-1", "5"],
      ],
      zero: 100,
    },
    {
      buckets: [
        [1, "-2", "-1", "5"],
        [3, "-1", "1", "5"],
        [0, "1", "2", "5"],
      ],
      zero: 50,
    },
  ])("positions the zero axis at $zero for $buckets", ({ buckets, zero }) => {
    const before = structuredClone(buckets);
    expect(getHistogramLayout(buckets, scale)?.zeroAxisLeft).toBe(zero);
    expect(buckets).toEqual(before);
  });

  it("caps infinite end buckets using the neighboring finite interval", () => {
    const layout = getHistogramLayout(
      [
        [1, "-Inf", "0", "5"],
        [0, "0", "10", "5"],
        [0, "10", "+Inf", "5"],
      ],
      scale,
    )!;
    expect(layout.bars).toHaveLength(3);
    for (let i = 0; i < 3; i++) {
      expect(layout.bars[i].left).toBeCloseTo((i * 100) / 3);
      expect(layout.bars[i].width).toBeCloseTo(100 / 3);
      expect(layout.bars[i].height).toBe(100);
    }
  });

  it.each<[Buckets]>([
    [
      [
        [1, "-8", "-2", "1"],
        [3, "-2", "4", "3"],
        [0, "4", "8", "2"],
      ],
    ],
    [
      [
        [1, "-2", "0", "1"],
        [0, "0", "8", "3"],
        [0, "8", "16", "2"],
      ],
    ],
  ])("keeps a reflected histogram symmetric for %j", (buckets) => {
    const reflected: Buckets = [...buckets]
      .reverse()
      .map(([rule, left, right, count]) => [
        rule,
        String(-Number(right)),
        String(-Number(left)),
        count,
      ]);
    const original = getHistogramLayout(buckets, scale)!;
    const mirror = getHistogramLayout(reflected, scale)!;
    original.bars.forEach((bar, i) => {
      const other = mirror.bars[mirror.bars.length - i - 1];
      expect(bar.width).toBeCloseTo(other.width);
      expect(bar.height).toBeCloseTo(other.height);
      expect(bar.left).toBeCloseTo(100 - other.left - other.width);
    });
    expect(original.zeroAxisLeft).toBeCloseTo(100 - mirror.zeroAxisLeft);
  });

  it("renders empty buckets with zero height", () => {
    const layout = getHistogramLayout(
      [
        [0, "0", "1", "0"],
        [0, "1", "2", "0"],
      ],
      scale,
    )!;
    expect(layout.bars.map((bar) => bar.height)).toEqual([0, 0]);
  });

  it.each<[Buckets]>([
    [[]],
    [[[0, "NaN", "1", "1"]]],
    [[[0, "2", "1", "1"]]],
    [
      [
        [0, "0", "2", "1"],
        [0, "1", "3", "1"],
      ],
    ],
    [[[0, "0", "1", "-1"]]],
    [[[0, "0", "1", "NaN"]]],
  ])("rejects invalid boundaries or counts in %j", (buckets) => {
    expect(getHistogramLayout(buckets, scale)).toBeNull();
  });
});

it("uses frequency density for linear bucket heights", () => {
  const layout = getHistogramLayout(
    [
      [0, "0", "1", "1"],
      [0, "1", "3", "1"],
    ],
    "linear",
  )!;
  expect(layout.bars[0].height).toBe(100);
  expect(layout.bars[1].height).toBeCloseTo(50);
});

it("uses counts for exponential bucket heights", () => {
  const layout = getHistogramLayout(
    [
      [0, "1", "2", "1"],
      [0, "2", "4", "3"],
    ],
    "exponential",
  )!;
  expect(layout.bars[0].height).toBeCloseTo(100 / 3);
  expect(layout.bars[1].height).toBe(100);
});

it("keeps original infinite boundaries in the bucket description", () => {
  expect(bucketRangeString([1, "-Inf", "0", "5"])).toBe("[-Inf -> 0)");
  expect(bucketRangeString([0, "0", "+Inf", "5"])).toBe("(0 -> +Inf]");
});

it("keeps exponential bar heights consistent with a zero-width bucket's count", () => {
  const layout = getHistogramLayout(
    [
      [1, "-1", "-0", "2"],
      [3, "-0", "0", "10"],
      [0, "0", "1", "2"],
    ],
    "exponential",
  )!;
  expect(layout.countMax).toBe(10);
  expect(layout.bars.map((bar) => bar.height)).toEqual([20, 0, 20]);
});
