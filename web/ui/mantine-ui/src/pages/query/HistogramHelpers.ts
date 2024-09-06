// Calculates a default width of exponential histogram bucket ranges. If the last bucket is [0, 0],
// the width is calculated using the second to last bucket. returns error if the last bucket is [-0, 0],
export function calculateDefaultExpBucketWidth(
  last: [number, string, string, string],
  buckets: [number, string, string, string][]
): number {
  if (parseFloat(last[2]) === 0 || parseFloat(last[1]) === 0) {
    if (buckets.length > 1) {
      return Math.abs(
        Math.log(Math.abs(parseFloat(buckets[buckets.length - 2][2]))) -
          Math.log(Math.abs(parseFloat(buckets[buckets.length - 2][1])))
      );
    } else {
      throw new Error(
        "Only one bucket in histogram ([-0, 0]). Cannot calculate defaultExpBucketWidth."
      );
    }
  } else {
    return Math.abs(
      Math.log(Math.abs(parseFloat(last[2]))) -
        Math.log(Math.abs(parseFloat(last[1])))
    );
  }
}

// Finds the lowest positive value from the bucket ranges
// Returns 0 if no positive values are found or if there are no buckets.
export function findMinPositive(buckets: [number, string, string, string][]) {
  if (!buckets || buckets.length === 0) {
    return 0; // no buckets
  }
  for (let i = 0; i < buckets.length; i++) {
    const right = parseFloat(buckets[i][2]);
    const left = parseFloat(buckets[i][1]);

    if (left > 0) {
      return left;
    }
    if (left < 0 && right > 0) {
      return right;
    }
    if (i === buckets.length - 1) {
      if (right > 0) {
        return right;
      }
    }
  }
  return 0; // all buckets are negative
}

// Finds the lowest negative value from the bucket ranges
// Returns 0 if no negative values are found or if there are no buckets.
export function findMaxNegative(buckets: [number, string, string, string][]) {
  if (!buckets || buckets.length === 0) {
    return 0; // no buckets
  }
  for (let i = 0; i < buckets.length; i++) {
    const right = parseFloat(buckets[i][2]);
    const left = parseFloat(buckets[i][1]);
    const prevRight = i > 0 ? parseFloat(buckets[i - 1][2]) : 0;

    if (right >= 0) {
      if (i === 0) {
        if (left < 0) {
          return left; // return the first negative bucket
        }
        return 0; // all buckets are positive
      }
      return prevRight; // return the last negative bucket
    }
  }
  console.log("findmaxneg returning: ", buckets[buckets.length - 1][2]);
  return parseFloat(buckets[buckets.length - 1][2]); // all buckets are negative
}

// Calculates the left position of the zero axis as a percentage string.
export function findZeroAxisLeft(
  scale: string,
  rangeMin: number,
  rangeMax: number,
  minPositive: number,
  maxNegative: number,
  zeroBucketIdx: number,
  widthNegative: number,
  widthTotal: number,
  expBucketWidth: number
): string {
  if (scale === "linear") {
    return ((0 - rangeMin) / (rangeMax - rangeMin)) * 100 + "%";
  } else {
    if (maxNegative === 0) {
      return "0%";
    }
    if (minPositive === 0) {
      return "100%";
    }
    if (zeroBucketIdx === -1) {
      // if there is no zero bucket, we must zero axis between buckets around zero
      return (widthNegative / widthTotal) * 100 + "%";
    }
    if ((widthNegative + 0.5 * expBucketWidth) / widthTotal > 0) {
      return ((widthNegative + 0.5 * expBucketWidth) / widthTotal) * 100 + "%";
    } else {
      return "0%";
    }
  }
}

// Determines if the zero axis should be shown such that the zero label does not overlap with the range labels.
// The zero axis is shown if it is between 5% and 95% of the graph.
export function showZeroAxis(zeroAxisLeft: string) {
  const axisNumber = parseFloat(zeroAxisLeft.slice(0, -1));
  if (5 < axisNumber && axisNumber < 95) {
    return true;
  }
  return false;
}

// Finds the index of the bucket whose range includes zero
export function findZeroBucket(
  buckets: [number, string, string, string][]
): number {
  for (let i = 0; i < buckets.length; i++) {
    const left = parseFloat(buckets[i][1]);
    const right = parseFloat(buckets[i][2]);
    if (left <= 0 && right >= 0) {
      return i;
    }
  }
  return -1;
}

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
