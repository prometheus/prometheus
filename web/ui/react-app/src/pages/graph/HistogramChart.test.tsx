import React from 'react';
import { render, screen } from '@testing-library/react';
import HistogramChart from './HistogramChart';
import { Histogram } from '../../types/types';

const mockFormat = jest.fn((value) => value.toString());
const mockResolvedOptions = jest.fn().mockReturnValue({ locale: 'en-US', numberingSystem: 'latn', style: 'decimal' });
const mockFormatToParts = jest.fn();
const mockFormatRange = jest.fn();
const mockFormatRangeToParts = jest.fn();

const histogramDataLinear: Histogram = {
  count: '30',
  sum: '350',
  buckets: [
    [1678886400, '0', '10', '5'],
    [1678886400, '10', '20', '15'],
    [1678886400, '20', '30', '10'],
  ],
};

const histogramDataExponential: Histogram = {
  count: '140',
  sum: '...',
  buckets: [
    [1678886400, '-100', '-10', '20'],
    [1678886400, '-10', '-1', '30'],
    [1678886400, '1', '10', '50'],
    [1678886400, '10', '100', '40'],
  ],
};

const histogramDataZeroCrossing: Histogram = {
  count: '30',
  sum: '...',
  buckets: [
    [1678886400, '-5', '-1', '10'],
    [1678886400, '-1', '1', '5'],
    [1678886400, '1', '5', '15'],
  ],
};

const bucketSlots = (container: HTMLElement): HTMLElement[] =>
  Array.from(container.querySelectorAll<HTMLElement>('.histogram-bucket-slot'));

describe('HistogramChart', () => {
  let numberFormatSpy: jest.SpyInstance;

  beforeAll(() => {
    numberFormatSpy = jest.spyOn(global.Intl, 'NumberFormat').mockImplementation(() => ({
      format: mockFormat,
      resolvedOptions: mockResolvedOptions,
      formatToParts: mockFormatToParts,
      formatRange: mockFormatRange,
      formatRangeToParts: mockFormatRangeToParts,
    }));
  });

  afterAll(() => {
    numberFormatSpy.mockRestore();
  });

  beforeEach(() => {
    mockFormat.mockClear();
  });

  it('renders no chart when buckets are empty or missing', () => {
    const empty: Histogram = { count: '0', sum: '0', buckets: [] };
    const { container, rerender } = render(<HistogramChart index={0} histogram={empty} scale="linear" />);

    expect(screen.getByText('No data')).toBeTruthy();
    expect(container.querySelector('.histogram-container')).toBeNull();

    rerender(
      <HistogramChart index={0} histogram={{ ...empty, buckets: null as unknown as Histogram['buckets'] }} scale="linear" />
    );
    expect(screen.getByText('No data')).toBeTruthy();
  });

  it('renders linear buckets, axes, and frequency-density styles', () => {
    const { container } = render(<HistogramChart index={0} histogram={histogramDataLinear} scale="linear" />);

    expect(container.querySelectorAll('.histogram-bucket')).toHaveLength(3);
    expect(container.querySelectorAll('.histogram-y-label')).toHaveLength(5);
    expect(container.querySelectorAll('.histogram-y-grid')).toHaveLength(5);
    expect(container.querySelectorAll('.histogram-x-grid')).toHaveLength(6);
    expect(mockFormat).toHaveBeenCalledWith(0);
    expect(mockFormat).toHaveBeenCalledWith(30);

    const slots = bucketSlots(container);
    expect(parseFloat(slots[0].style.left)).toBeCloseTo(0, 1);
    expect(parseFloat(slots[0].style.width)).toBeCloseTo(100 / 3, 1);
    expect(parseFloat((slots[0].querySelector('.histogram-bucket') as HTMLElement).style.height)).toBeCloseTo(100 / 3, 1);
    expect(parseFloat(slots[1].style.left)).toBeCloseTo(100 / 3, 1);
    expect((slots[1].querySelector('.histogram-bucket') as HTMLElement).style.height).toBe('100%');
    expect(parseFloat(slots[2].style.left)).toBeCloseTo((2 * 100) / 3, 1);
    expect(parseFloat((slots[2].querySelector('.histogram-bucket') as HTMLElement).style.height)).toBeCloseTo(
      (2 * 100) / 3,
      1
    );
  });

  it('renders exponential buckets and count-based heights', () => {
    const { container } = render(<HistogramChart index={1} histogram={histogramDataExponential} scale="exponential" />);

    expect(container.querySelectorAll('.histogram-bucket')).toHaveLength(4);
    expect(mockFormat).toHaveBeenCalledWith(50);
    expect(mockFormat).toHaveBeenCalledWith(37.5);
    expect(mockFormat).toHaveBeenCalledWith(-100);
    expect(mockFormat).toHaveBeenCalledWith(100);

    const slots = bucketSlots(container);
    expect((slots[0].querySelector('.histogram-bucket') as HTMLElement).style.height).toBe('40%');
    expect((slots[1].querySelector('.histogram-bucket') as HTMLElement).style.height).toBe('60%');
    expect((slots[2].querySelector('.histogram-bucket') as HTMLElement).style.height).toBe('100%');
    expect((slots[3].querySelector('.histogram-bucket') as HTMLElement).style.height).toBe('80%');
    slots.forEach((slot) => {
      expect(parseFloat(slot.style.left)).toBeGreaterThanOrEqual(0);
      expect(parseFloat(slot.style.width)).toBeGreaterThan(0);
    });
    expect(parseFloat(slots[3].style.left) + parseFloat(slots[3].style.width)).toBeLessThanOrEqual(100.01);
  });

  it('positions a zero-crossing exponential bucket', () => {
    const { container } = render(<HistogramChart index={2} histogram={histogramDataZeroCrossing} scale="exponential" />);
    const zeroCrossing = bucketSlots(container)[1];

    expect(parseFloat(zeroCrossing.style.left)).toBeGreaterThanOrEqual(0);
    expect(parseFloat(zeroCrossing.style.width)).toBeGreaterThan(0);
    expect(parseFloat((zeroCrossing.querySelector('.histogram-bucket') as HTMLElement).style.height)).toBeCloseTo(
      100 / 3,
      1
    );
  });
});
