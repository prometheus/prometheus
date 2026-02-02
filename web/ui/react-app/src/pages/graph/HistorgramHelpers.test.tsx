import {
  calculateDefaultExpBucketWidth,
  findMinPositive,
  findMaxNegative,
  findZeroBucket,
  findZeroAxisLeft,
  showZeroAxis,
} from './HistogramHelpers';

type Bucket = [number, string, string, string];

describe('HistogramHelpers', () => {
  const bucketsAllPositive: Bucket[] = [
    [0, '1', '10', '5'],
    [0, '10', '100', '15'],
    [0, '100', '1000', '10'],
  ];

  const bucketsAllNegative: Bucket[] = [
    [0, '-1000', '-100', '10'],
    [0, '-100', '-10', '15'],
    [0, '-10', '-1', '5'],
  ];

  const bucketsCrossingZeroMid: Bucket[] = [
    [0, '-100', '-10', '10'],
    [0, '-10', '-1', '15'],
    [0, '-1', '1', '5'],
    [0, '1', '10', '20'],
    [0, '10', '100', '8'],
  ];

  const bucketsWithExactZeroBucket: Bucket[] = [
    [0, '-10', '-1', '15'],
    [0, '0', '0', '5'],
    [0, '1', '10', '20'],
  ];

  const bucketsStartingWithZeroCross: Bucket[] = [
    [0, '-1', '1', '5'],
    [0, '1', '10', '20'],
    [0, '10', '100', '8'],
  ];

  const bucketsEndingWithZeroCross: Bucket[] = [
    [0, '-100', '-10', '10'],
    [0, '-10', '-1', '15'],
    [0, '-1', '1', '5'],
  ];

  const singleZeroBucket: Bucket[] = [[0, '0', '0', '10']];

  const emptyBuckets: Bucket[] = [];

  const bucketsWithZeroFallback: Bucket[] = [
    [0, '1', '10', '5'],
    [0, '10', '100', '15'],
    [0, '0', '0', '2'],
  ];

  const bucketsNegThenPosNoCross: Bucket[] = [
    [0, '-10', '-1', '15'],
    [0, '5', '10', '20'],
  ];

  describe('calculateDefaultExpBucketWidth', () => {
    it('calculates width for a standard positive bucket', () => {
      const lastBucket = bucketsAllPositive[bucketsAllPositive.length - 1];
      const expected = Math.log(1000) - Math.log(100);
      expect(calculateDefaultExpBucketWidth(lastBucket, bucketsAllPositive)).toBeCloseTo(expected);
    });

    it('calculates width for a standard negative bucket', () => {
      const lastBucket = bucketsAllNegative[bucketsAllNegative.length - 1];
      const expectedAbs = Math.abs(
        Math.log(Math.abs(parseFloat(lastBucket[2]))) - Math.log(Math.abs(parseFloat(lastBucket[1])))
      );
      expect(calculateDefaultExpBucketWidth(lastBucket, bucketsAllNegative)).toBeCloseTo(expectedAbs);
    });

    it('uses the previous bucket if the last bucket is [0, 0]', () => {
      const lastBucket = bucketsWithZeroFallback[bucketsWithZeroFallback.length - 1];
      const expected = Math.log(100) - Math.log(10);
      expect(calculateDefaultExpBucketWidth(lastBucket, bucketsWithZeroFallback)).toBeCloseTo(expected);
    });

    it('throws an error if only a single [0, 0] bucket exists', () => {
      const lastBucket = singleZeroBucket[0];
      expect(() => calculateDefaultExpBucketWidth(lastBucket, singleZeroBucket)).toThrow(
        'Only one bucket in histogram ([-0, 0]). Cannot calculate defaultExpBucketWidth.'
      );
    });
  });

  describe('findMinPositive', () => {
    it('returns the first positive left bound when all are positive', () => {
      expect(findMinPositive(bucketsAllPositive)).toEqual(1);
    });

    it('returns the left bound when it is the first positive value', () => {
      expect(findMinPositive(bucketsNegThenPosNoCross)).toBe(5);
    });

    it('returns the right bound when left is negative and right is positive (middle cross)', () => {
      expect(findMinPositive(bucketsCrossingZeroMid)).toBe(1);
    });

    it('returns the right bound when the first bucket crosses zero', () => {
      expect(findMinPositive(bucketsStartingWithZeroCross)).toBe(1);
    });

    it('returns the right bound when the last bucket crosses zero', () => {
      expect(findMinPositive(bucketsEndingWithZeroCross)).toBe(1);
    });

    it('returns 0 when all buckets are negative', () => {
      expect(findMinPositive(bucketsAllNegative)).toBe(0);
    });

    it('returns 0 for empty buckets', () => {
      expect(findMinPositive(emptyBuckets)).toBe(0);
    });

    it('returns 0 for only zero bucket', () => {
      expect(findMinPositive(singleZeroBucket)).toBe(0);
    });

    it('returns 0 when buckets is undefined', () => {
      expect(findMinPositive(undefined as any)).toBe(0);
    });

    it('returns the correct positive bound with exact zero bucket present', () => {
      expect(findMinPositive(bucketsWithExactZeroBucket)).toBe(1);
    });
  });

  describe('findMaxNegative', () => {
    it('returns 0 when all buckets are positive', () => {
      expect(findMaxNegative(bucketsAllPositive)).toBe(0);
    });

    it('returns the right bound of the last negative bucket when all are negative', () => {
      expect(findMaxNegative(bucketsAllNegative)).toEqual(-1);
    });

    it('returns the right bound of the bucket before the middle zero-crossing bucket', () => {
      expect(findMaxNegative(bucketsCrossingZeroMid)).toEqual(-1);
    });

    it('returns the left bound when the first bucket crosses zero', () => {
      expect(findMaxNegative(bucketsStartingWithZeroCross)).toBe(-1);
    });

    it('returns the right bound of the bucket before the last zero-crossing bucket', () => {
      expect(findMaxNegative(bucketsEndingWithZeroCross)).toEqual(-1);
    });

    it('returns 0 for empty buckets', () => {
      expect(findMaxNegative(emptyBuckets)).toBe(0);
    });

    it('returns 0 for only zero bucket', () => {
      expect(findMaxNegative(singleZeroBucket)).toBe(0);
    });

    it('returns 0 when buckets is undefined', () => {
      expect(findMaxNegative(undefined as any)).toBe(0);
    });

    it('returns the right bound of the bucket before an exact zero bucket', () => {
      expect(findMaxNegative(bucketsWithExactZeroBucket)).toEqual(-1);
    });
  });

  describe('findZeroBucket', () => {
    it('returns the index of bucket strictly containing zero', () => {
      expect(findZeroBucket(bucketsCrossingZeroMid)).toBe(2);
    });

    it('returns the index of bucket with zero as left boundary', () => {
      const buckets: Bucket[] = [
        [0, '-5', '-1', '10'],
        [0, '0', '5', '15'],
      ];
      expect(findZeroBucket(buckets)).toBe(1);
    });

    it('returns the index of bucket with zero as right boundary', () => {
      const buckets: Bucket[] = [
        [0, '-5', '0', '10'],
        [0, '1', '5', '15'],
      ];
      expect(findZeroBucket(buckets)).toBe(0);
    });

    it('returns the index of an exact [0, 0] bucket', () => {
      expect(findZeroBucket(bucketsWithExactZeroBucket)).toBe(1);
    });

    it('returns -1 when there is a gap around zero', () => {
      expect(findZeroBucket(bucketsNegThenPosNoCross)).toBe(-1);
    });

    it('returns -1 when all buckets are positive', () => {
      expect(findZeroBucket(bucketsAllPositive)).toBe(-1);
    });

    it('returns -1 when all buckets are negative', () => {
      expect(findZeroBucket(bucketsAllNegative)).toBe(-1);
    });

    it('returns 0 if the first bucket crosses zero', () => {
      expect(findZeroBucket(bucketsStartingWithZeroCross)).toBe(0);
    });

    it('returns the last index if the last bucket crosses zero', () => {
      expect(findZeroBucket(bucketsEndingWithZeroCross)).toBe(2);
    });

    it('returns -1 when buckets array is empty', () => {
      expect(findZeroBucket(emptyBuckets)).toBe(-1);
    });
  });

  describe('findZeroAxisLeft', () => {
    it('calculates correctly for linear scale crossing zero', () => {
      const rangeMin = -100;
      const rangeMax = 100;
      const expected = '50%';
      const result = findZeroAxisLeft('linear', rangeMin, rangeMax, 1, -1, 2, 0, 0, 0);
      expect(result).toEqual(expected);
    });

    it('calculates correctly for asymmetric linear scale crossing zero', () => {
      const rangeMin = -10;
      const rangeMax = 90;
      const expectedNumber = ((0 - rangeMin) / (rangeMax - rangeMin)) * 100;
      const resultString = findZeroAxisLeft('linear', rangeMin, rangeMax, 1, -1, 0, 0, 0, 0);
      expect(parseFloat(resultString)).toBeCloseTo(expectedNumber, 1);
    });

    it('calculates correctly for linear scale all positive (off-scale left)', () => {
      const rangeMin = 10;
      const rangeMax = 100;
      const expectedNumber = ((0 - rangeMin) / (rangeMax - rangeMin)) * 100;
      const resultString = findZeroAxisLeft('linear', rangeMin, rangeMax, 10, 0, -1, 0, 0, 0);
      expect(parseFloat(resultString)).toBeCloseTo(expectedNumber, 1);
    });

    it('calculates correctly for linear scale all negative (off-scale right)', () => {
      const rangeMin = -100;
      const rangeMax = -10;
      const expectedNumber = ((0 - rangeMin) / (rangeMax - rangeMin)) * 100;
      const resultString = findZeroAxisLeft('linear', rangeMin, rangeMax, 0, -10, -1, 0, 0, 0);
      expect(parseFloat(resultString)).toBeCloseTo(expectedNumber, 1);
    });

    const expMinPos = 1;
    const expMaxNeg = -1;
    const expZeroIdx = 2;
    const defaultExpBW = Math.log(10);
    const expNegWidth = Math.abs(Math.log(Math.abs(-1)) - Math.log(Math.abs(-100)));
    const expPosWidth = Math.log(100) - Math.log(1);
    const expTotalWidth = expNegWidth + expPosWidth + defaultExpBW;

    it('returns 0% for exponential scale when maxNegative is 0', () => {
      expect(findZeroAxisLeft('exponential', 1, 100, 1, 0, -1, 0, expPosWidth + defaultExpBW, defaultExpBW)).toEqual('0%');
    });

    it('returns 100% for exponential scale when minPositive is 0', () => {
      expect(
        findZeroAxisLeft('exponential', -100, -1, 0, -1, -1, expNegWidth, expNegWidth + defaultExpBW, defaultExpBW)
      ).toEqual('100%');
    });

    it('calculates position between buckets when zeroBucketIdx is -1 (exponential)', () => {
      const minPos = 5;
      const maxNeg = -1;
      const zeroIdx = -1;
      const negW = Math.log(Math.abs(-1)) - Math.log(Math.abs(-10));
      const posW = Math.log(10) - Math.log(5);
      const totalW = Math.abs(negW) + posW + defaultExpBW;
      const expectedNumber = (Math.abs(negW) / totalW) * 100;
      const resultString = findZeroAxisLeft(
        'exponential',
        -10,
        10,
        minPos,
        maxNeg,
        zeroIdx,
        Math.abs(negW),
        totalW,
        defaultExpBW
      );
      expect(parseFloat(resultString)).toBeCloseTo(expectedNumber, 1);
    });

    it('calculates position using bucket width when zeroBucketIdx exists (exponential)', () => {
      const expectedNumber = ((expNegWidth + 0.5 * defaultExpBW) / expTotalWidth) * 100;
      const resultString = findZeroAxisLeft(
        'exponential',
        -100,
        100,
        expMinPos,
        expMaxNeg,
        expZeroIdx,
        expNegWidth,
        expTotalWidth,
        defaultExpBW
      );
      expect(parseFloat(resultString)).toBeCloseTo(expectedNumber, 1);
    });

    it('returns 0% for exponential when calculation is negative (edge case)', () => {
      expect(findZeroAxisLeft('exponential', -10, 20, 1, -5, 1, -5, 15, 2)).toBe('0%');
    });
  });

  describe('showZeroAxis', () => {
    it('returns true when axis is between 5% and 95%', () => {
      expect(showZeroAxis('5.01%')).toBe(true);
      expect(showZeroAxis('50%')).toBe(true);
      expect(showZeroAxis('94.99%')).toBe(true);
    });

    it('returns false when axis is less than or equal to 5%', () => {
      expect(showZeroAxis('5%')).toBe(false);
      expect(showZeroAxis('0%')).toBe(false);
      expect(showZeroAxis('-10%')).toBe(false);
    });

    it('returns false when axis is greater than or equal to 95%', () => {
      expect(showZeroAxis('95%')).toBe(false);
      expect(showZeroAxis('100%')).toBe(false);
      expect(showZeroAxis('120%')).toBe(false);
    });
  });
});
