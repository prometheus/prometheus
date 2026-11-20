import { Histogram } from "../../types/types";

type Bucket = NonNullable<Histogram["buckets"]>[number];

const parseBoundary = (value: string): number => {
  if (value === "-Inf") {
    return -Infinity;
  }
  if (value === "+Inf" || value === "Inf") {
    return Infinity;
  }
  return Number(value);
};

// Projects bucket boundaries onto one continuous axis. Exponential scales are
// linear around zero, and infinite end buckets use the adjacent finite width.
// The input boundaries remain unchanged for labels and tooltips.
export const getHistogramLayout = (buckets: Bucket[], scale: string) => {
  if (scale !== "linear" && scale !== "exponential") {
    throw new Error("Invalid scale");
  }
  const values = buckets.map(([, left, right, count]) => ({
    left: parseBoundary(left),
    right: parseBoundary(right),
    count: Number(count),
  }));
  if (
    values.length === 0 ||
    values.some(
      (b, i) =>
        Number.isNaN(b.left) ||
        Number.isNaN(b.right) ||
        b.left === Infinity ||
        b.right === -Infinity ||
        b.left > b.right ||
        !Number.isFinite(b.count) ||
        b.count < 0 ||
        (i > 0 && b.left < values[i - 1].right) ||
        (i > 0 && b.left === -Infinity) ||
        (i < values.length - 1 && b.right === Infinity),
    )
  ) {
    return null;
  }

  const finiteBounds = values
    .flatMap((b) => [b.left, b.right])
    .filter(Number.isFinite);
  const nonZeroBounds = finiteBounds.filter((v) => v !== 0).map(Math.abs);
  const closest = nonZeroBounds.length ? Math.min(...nonZeroBounds) : 1;
  const zeroBounds = values
    .filter((b) => b.left <= 0 && b.right >= 0)
    .flatMap((b) => [b.left, b.right])
    .filter(Number.isFinite)
    .map(Math.abs);
  const threshold = Math.max(...zeroBounds, 0) || closest;
  const logWidths = values
    .filter(
      (b) =>
        Number.isFinite(b.left) &&
        Number.isFinite(b.right) &&
        ((b.left > 0 && b.right > 0) || (b.left < 0 && b.right < 0)),
    )
    .map((b) =>
      Math.abs(Math.log(Math.abs(b.right)) - Math.log(Math.abs(b.left))),
    )
    .filter((width) => width > 0);
  const zeroHalfWidth =
    logWidths.length === 0
      ? 0.5
      : logWidths.reduce((sum, width) => sum + width, 0) / logWidths.length / 2;
  const linearScale = Math.max(...finiteBounds.map(Math.abs), 1);
  const project = (v: number): number => {
    if (scale === "linear") {
      return v / linearScale;
    }
    const absolute = Math.abs(v);
    if (absolute <= threshold) {
      return (v / threshold) * zeroHalfWidth;
    }
    return (
      Math.sign(v) * (zeroHalfWidth + Math.log(absolute) - Math.log(threshold))
    );
  };
  const ranges = values.map((b) => ({
    left: project(b.left),
    right: project(b.right),
  }));
  const finiteWidths = ranges
    .filter(
      (b) =>
        Number.isFinite(b.left) && Number.isFinite(b.right) && b.right > b.left,
    )
    .map((b) => b.right - b.left);
  const points = ranges
    .flatMap((b) => [b.left, b.right])
    .filter(Number.isFinite);
  if (ranges[0].left === -Infinity) {
    ranges[0].left = (points[0] ?? 0) - (finiteWidths[0] ?? 1);
  }
  if (ranges[ranges.length - 1].right === Infinity) {
    ranges[ranges.length - 1].right =
      (points[points.length - 1] ?? 0) +
      (finiteWidths[finiteWidths.length - 1] ?? 1);
  }
  // A sole zero-width bucket still represents observations at zero.
  if (values.length === 1 && values[0].left === 0 && values[0].right === 0) {
    ranges[0] = { left: -0.5, right: 0.5 };
  }
  const min = ranges[0].left;
  const max = ranges[ranges.length - 1].right;
  const width = max - min;
  if (!Number.isFinite(width) || width <= 0) {
    return null;
  }
  const countMax = Math.max(...values.map((b) => b.count));
  const widths = ranges.map((b) => b.right - b.left);
  const positiveWidths = widths.filter((w) => w > 0);
  const minWidth = positiveWidths.length ? Math.min(...positiveWidths) : 1;
  // Normalize before dividing to avoid overflow for narrow or populous buckets.
  const heights = values.map((b, i) =>
    countMax === 0 || widths[i] === 0
      ? 0
      : (b.count / countMax) * (scale === "linear" ? minWidth / widths[i] : 1),
  );
  const heightMax = scale === "linear" ? Math.max(...heights) : 1;
  return {
    countMax,
    zeroAxisLeft: Math.max(0, Math.min(100, ((0 - min) / width) * 100)),
    bars: ranges.map((b, i) => ({
      left: ((b.left - min) / width) * 100,
      width: (widths[i] / width) * 100,
      height: heightMax === 0 ? 0 : (heights[i] / heightMax) * 100,
    })),
  };
};

const leftDelim = (br: number): string => (br === 3 || br === 1 ? "[" : "(");
const rightDelim = (br: number): string => (br === 3 || br === 0 ? "]" : ")");

export const bucketRangeString = ([
  boundaryRule,
  leftBoundary,
  rightBoundary,

  _,
]: [number, string, string, string]): string => {
  return `${leftDelim(boundaryRule)}${leftBoundary} -> ${rightBoundary}${rightDelim(boundaryRule)}`;
};
