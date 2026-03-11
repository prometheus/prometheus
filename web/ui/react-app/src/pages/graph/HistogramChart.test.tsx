import React from 'react';
import { mount, ReactWrapper } from 'enzyme';
import HistogramChart from './HistogramChart';
import { Histogram } from '../../types/types';

const mockFormat = jest.fn((value) => value.toString());
const mockResolvedOptions = jest.fn().mockReturnValue({ locale: 'en-US', numberingSystem: 'latn', style: 'decimal' });
const mockFormatToParts = jest.fn();
const mockFormatRange = jest.fn();
const mockFormatRangeToParts = jest.fn();

jest.spyOn(global.Intl, 'NumberFormat').mockImplementation(() => ({
  format: mockFormat,
  resolvedOptions: mockResolvedOptions,
  formatToParts: mockFormatToParts,
  formatRange: mockFormatRange,
  formatRangeToParts: mockFormatRangeToParts,
}));

describe('HistogramChart', () => {
  let wrapper: ReactWrapper;

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

  const histogramDataEmpty: Histogram = {
    count: '0',
    sum: '0',
    buckets: [],
  };

  const histogramDataNull: Histogram = {
    count: '0',
    sum: '0',
    buckets: null as any,
  };

  const defaultProps = {
    index: 0,
    scale: 'linear' as 'linear' | 'exponential',
  };

  beforeEach(() => {
    mockFormat.mockClear();
    mockResolvedOptions.mockClear();
    mockFormatToParts.mockClear();
    mockFormatRange.mockClear();
    mockFormatRangeToParts.mockClear();
  });

  afterEach(() => {
    if (wrapper && wrapper.exists()) {
      wrapper.unmount();
    }
  });

  it('renders without crashing', () => {
    wrapper = mount(<HistogramChart {...defaultProps} histogram={histogramDataLinear} scale="linear" />);
    expect(wrapper.find('.histogram-y-wrapper').exists()).toBe(true);
    expect(wrapper.find('.histogram-container').exists()).toBe(true);
  });

  it('renders "No data" when buckets are empty', () => {
    wrapper = mount(<HistogramChart {...defaultProps} histogram={histogramDataEmpty} scale="linear" />);
    expect(wrapper.text()).toContain('No data');
    expect(wrapper.find('.histogram-container').exists()).toBe(false);
  });

  it('renders "No data" when buckets are null', () => {
    wrapper = mount(<HistogramChart {...defaultProps} histogram={histogramDataNull} scale="linear" />);
    expect(wrapper.text()).toContain('No data');
    expect(wrapper.find('.histogram-container').exists()).toBe(false);
  });

  describe('Linear Scale', () => {
    beforeEach(() => {
      wrapper = mount(<HistogramChart {...defaultProps} histogram={histogramDataLinear} scale="linear" />);
    });

    it('renders the correct number of buckets', () => {
      expect(wrapper.find('.histogram-bucket')).toHaveLength(histogramDataLinear.buckets!.length);
    });

    it('renders y-axis labels and grid lines', () => {
      expect(wrapper.find('.histogram-y-label')).toHaveLength(5);
      expect(wrapper.find('.histogram-y-grid')).toHaveLength(5);
      expect(wrapper.find('.histogram-y-tick')).toHaveLength(5);
      expect(wrapper.find('.histogram-y-label').at(0).text()).toBe('');
      expect(wrapper.find('.histogram-y-label').last().text()).toBe('0');
    });

    it('renders x-axis labels and grid lines', () => {
      expect(wrapper.find('.histogram-x-label')).toHaveLength(1);
      expect(wrapper.find('.histogram-x-grid')).toHaveLength(6);
      expect(wrapper.find('.histogram-x-tick')).toHaveLength(3);
      expect(mockFormat).toHaveBeenCalledWith(0);
      expect(mockFormat).toHaveBeenCalledWith(30);
      expect(wrapper.find('.histogram-x-label').text()).toContain('0');
      expect(wrapper.find('.histogram-x-label').text()).toContain('30');
    });

    it('calculates bucket styles correctly for linear scale', () => {
      const buckets = wrapper.find('.histogram-bucket-slot');
      const rangeMin = 0;
      const rangeMax = 30;
      const rangeWidth = rangeMax - rangeMin;
      const fdMax = 1.5;

      const b1 = buckets.at(0);
      const expectedB1LeftNum = ((0 - rangeMin) / rangeWidth) * 100;
      const expectedB1WidthNum = ((10 - 0) / rangeWidth) * 100;
      const expectedB1HeightNum = (0.5 / fdMax) * 100;
      expect(parseFloat(b1.prop('style')?.left as string)).toBeCloseTo(expectedB1LeftNum, 1);
      expect(parseFloat(b1.prop('style')?.width as string)).toBeCloseTo(expectedB1WidthNum, 1);
      expect(parseFloat(b1.find('.histogram-bucket').prop('style')?.height as string)).toBeCloseTo(expectedB1HeightNum, 1);

      const b2 = buckets.at(1);
      const expectedB2LeftNum = ((10 - rangeMin) / rangeWidth) * 100;
      const expectedB2WidthNum = ((20 - 10) / rangeWidth) * 100;
      const expectedB2HeightNum = (1.5 / fdMax) * 100;
      expect(parseFloat(b2.prop('style')?.left as string)).toBeCloseTo(expectedB2LeftNum, 1);
      expect(parseFloat(b2.prop('style')?.width as string)).toBeCloseTo(expectedB2WidthNum, 1);
      expect(parseFloat(b2.find('.histogram-bucket').prop('style')?.height as string)).toBeCloseTo(expectedB2HeightNum, 1);

      const b3 = buckets.at(2);
      const expectedB3LeftNum = ((20 - rangeMin) / rangeWidth) * 100;
      const expectedB3WidthNum = ((30 - 20) / rangeWidth) * 100;
      const expectedB3HeightNum = (1.0 / fdMax) * 100;
      expect(parseFloat(b3.prop('style')?.left as string)).toBeCloseTo(expectedB3LeftNum, 1);
      expect(parseFloat(b3.prop('style')?.width as string)).toBeCloseTo(expectedB3WidthNum, 1);
      expect(parseFloat(b3.find('.histogram-bucket').prop('style')?.height as string)).toBeCloseTo(expectedB3HeightNum, 1);
    });
  });

  describe('Exponential Scale', () => {
    beforeEach(() => {
      wrapper = mount(
        <HistogramChart {...defaultProps} index={1} histogram={histogramDataExponential} scale="exponential" />
      );
    });

    it('renders the correct number of buckets', () => {
      expect(wrapper.find('.histogram-bucket')).toHaveLength(histogramDataExponential.buckets!.length);
    });

    it('renders y-axis labels and grid lines with formatting', () => {
      expect(wrapper.find('.histogram-y-label')).toHaveLength(5);
      expect(wrapper.find('.histogram-y-grid')).toHaveLength(5);
      expect(wrapper.find('.histogram-y-tick')).toHaveLength(5);

      const countMax = 50;
      expect(mockFormat).toHaveBeenCalledWith(countMax * 1);
      expect(mockFormat).toHaveBeenCalledWith(countMax * 0.75);
      expect(mockFormat).toHaveBeenCalledWith(countMax * 0.5);
      expect(mockFormat).toHaveBeenCalledWith(countMax * 0.25);

      expect(wrapper.find('.histogram-y-label').at(0).text()).toBe('50');
      expect(wrapper.find('.histogram-y-label').at(1).text()).toBe('37.5');
      expect(wrapper.find('.histogram-y-label').last().text()).toBe('0');
    });

    it('renders x-axis labels and grid lines with formatting', () => {
      expect(wrapper.find('.histogram-x-label')).toHaveLength(1);
      expect(wrapper.find('.histogram-x-grid')).toHaveLength(6);
      expect(wrapper.find('.histogram-x-tick')).toHaveLength(3);

      expect(mockFormat).toHaveBeenCalledWith(-100);
      expect(mockFormat).toHaveBeenCalledWith(100);
      expect(wrapper.find('.histogram-x-label').text()).toContain('0');
      expect(wrapper.find('.histogram-x-label').text()).toContain('-100');
      expect(wrapper.find('.histogram-x-label').text()).toContain('100');
    });

    it('calculates bucket styles correctly for exponential scale', () => {
      const buckets = wrapper.find('.histogram-bucket-slot');
      const countMax = 50;

      const b1 = buckets.at(0);
      const b1Height = (20 / countMax) * 100;
      expect(b1.find('.histogram-bucket').prop('style')).toHaveProperty('height', `${b1Height}%`);
      expect(parseFloat(b1.prop('style')?.left as string)).toBeGreaterThanOrEqual(0);
      expect(parseFloat(b1.prop('style')?.width as string)).toBeGreaterThan(0);

      const b2 = buckets.at(1);
      const b2Height = (30 / countMax) * 100;
      expect(b2.find('.histogram-bucket').prop('style')).toHaveProperty('height', `${b2Height}%`);
      expect(parseFloat(b2.prop('style')?.left as string)).toBeGreaterThan(0);
      expect(parseFloat(b2.prop('style')?.width as string)).toBeGreaterThan(0);

      const b3 = buckets.at(2);
      const b3Height = (50 / countMax) * 100;
      expect(b3.find('.histogram-bucket').prop('style')).toHaveProperty('height', '100%');
      expect(parseFloat(b3.prop('style')?.left as string)).toBeGreaterThan(0);
      expect(parseFloat(b3.prop('style')?.width as string)).toBeGreaterThan(0);

      const b4 = buckets.at(3);
      const b4Height = (40 / countMax) * 100;
      expect(b4.find('.histogram-bucket').prop('style')).toHaveProperty('height', `${b4Height}%`);
      expect(parseFloat(b4.prop('style')?.left as string)).toBeGreaterThan(0);
      expect(parseFloat(b4.prop('style')?.width as string)).toBeGreaterThan(0);
      expect(
        parseFloat(b4.prop('style')?.left as string) + parseFloat(b4.prop('style')?.width as string)
      ).toBeLessThanOrEqual(100.01);
    });

    it('handles zero-crossing bucket correctly in exponential scale', () => {
      wrapper = mount(
        <HistogramChart {...defaultProps} index={2} histogram={histogramDataZeroCrossing} scale="exponential" />
      );
      const buckets = wrapper.find('.histogram-bucket-slot');
      const countMax = 15;

      const b2 = buckets.at(1);
      const b2Height = (5 / countMax) * 100;
      expect(b2.find('.histogram-bucket').prop('style')).toHaveProperty(
        'height',
        expect.stringContaining(b2Height.toFixed(1))
      );
      expect(parseFloat(b2.prop('style')?.left as string)).toBeGreaterThanOrEqual(0);
      expect(parseFloat(b2.prop('style')?.width as string)).toBeGreaterThan(0);
    });
  });
});
