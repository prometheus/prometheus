import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import $ from 'jquery';
import Graph, { GraphProps, GraphSeries } from './Graph';
import { GraphDisplayMode } from './Panel';

jest.mock('react-resize-detector', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({ onResize }: { onResize?: () => void }) =>
      React.createElement('button', { type: 'button', 'data-testid': 'resize-detector', onClick: onResize }, 'Resize'),
  };
});

const data: GraphProps['data'] = {
  resultType: 'matrix',
  result: [
    {
      metric: { job: 'prometheus', code: '200' },
      values: [
        [1572128592, '23'],
        [1572128620, '2'],
      ],
    },
    {
      metric: { job: 'prometheus', code: '500' },
      values: [
        [1572128592, '1'],
        [1572128620, '4'],
      ],
    },
  ],
};

const defaultProps: GraphProps = {
  queryParams: {
    startTime: 1572128592,
    endTime: 1572128692,
    resolution: 28,
  },
  displayMode: GraphDisplayMode.Stacked,
  data,
  exemplars: undefined,
  useLocalTime: false,
  showExemplars: false,
  handleTimeRangeSelection: jest.fn(),
  id: 'test',
};

describe('Graph', () => {
  const originalResizeObserver = window.ResizeObserver;
  let plot: {
    setData: jest.Mock;
    draw: jest.Mock;
    destroy: jest.Mock;
    getData: jest.Mock;
    clearSelection: jest.Mock;
  };
  let plotSpy: jest.SpyInstance;
  let animationFrameSpy: jest.SpyInstance;
  let cancelAnimationFrameSpy: jest.SpyInstance;

  beforeEach(() => {
    plot = {
      setData: jest.fn(),
      draw: jest.fn(),
      destroy: jest.fn(),
      getData: jest.fn().mockReturnValue([]),
      clearSelection: jest.fn(),
    };
    plotSpy = jest.spyOn($, 'plot').mockReturnValue(plot as unknown as jquery.flot.plot);
    animationFrameSpy = jest.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0);
      return 1;
    });
    cancelAnimationFrameSpy = jest.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined);
    window.ResizeObserver = jest.fn().mockImplementation(() => ({
      observe: jest.fn(),
      unobserve: jest.fn(),
      disconnect: jest.fn(),
    }));
    defaultProps.handleTimeRangeSelection = jest.fn();
  });

  afterEach(() => {
    plotSpy.mockRestore();
    animationFrameSpy.mockRestore();
    cancelAnimationFrameSpy.mockRestore();
    window.ResizeObserver = originalResizeObserver;
  });

  it('plots normalized data and updates stacking when props change', () => {
    const { container, rerender } = render(<Graph {...defaultProps} />);

    expect(container.querySelector('.graph-test .graph-chart')).toBeTruthy();
    expect(container.querySelectorAll('.legend-item')).toHaveLength(2);
    const initialSeries = plotSpy.mock.calls[0][1] as GraphSeries[];
    expect(initialSeries[0]).toMatchObject({ labels: { job: 'prometheus', code: '200' }, stack: true });
    expect(initialSeries[0].data.slice(0, 2)).toEqual([
      [1572128592000, 23],
      [1572128620000, 2],
    ]);

    const updatedData: GraphProps['data'] = {
      ...data,
      result: [{ metric: { job: 'prometheus', code: '200' }, values: [[1572128592, '7']] }],
    };
    rerender(<Graph {...defaultProps} data={updatedData} />);
    let latestSeries = plotSpy.mock.calls[plotSpy.mock.calls.length - 1][1] as GraphSeries[];
    expect(latestSeries[0].data[0]).toEqual([1572128592000, 7]);

    rerender(<Graph {...defaultProps} data={updatedData} displayMode={GraphDisplayMode.Lines} />);
    latestSeries = plotSpy.mock.calls[plotSpy.mock.calls.length - 1][1] as GraphSeries[];
    expect(latestSeries[0]).toMatchObject({ stack: false });
  });

  it('redraws highlighted series on legend hover and restores them on mouse out', () => {
    const { container } = render(<Graph {...defaultProps} />);

    fireEvent.mouseOver(container.querySelectorAll('.legend-item')[0]);
    expect(animationFrameSpy).toHaveBeenCalledTimes(1);
    expect(plot.setData).toHaveBeenCalledTimes(1);
    expect(plot.draw).toHaveBeenCalledTimes(1);

    fireEvent.mouseOut(container.querySelector('.graph-legend') as HTMLElement);
    expect(cancelAnimationFrameSpy).toHaveBeenCalled();
    expect(plot.setData).toHaveBeenCalledTimes(2);
    expect(plot.draw).toHaveBeenCalledTimes(2);
  });

  it('handles range selection, resize, and plot destruction', () => {
    const { unmount } = render(<Graph {...defaultProps} />);

    $('.graph-test').trigger('plotselected', [{ xaxis: { from: 100, to: 200 } }]);
    expect(plot.clearSelection).toHaveBeenCalledTimes(1);
    expect(defaultProps.handleTimeRangeSelection).toHaveBeenCalledWith(100, 200);

    fireEvent.click(screen.getByTestId('resize-detector'));
    expect(plot.getData).toHaveBeenCalledTimes(1);
    expect(plotSpy).toHaveBeenCalledTimes(2);

    unmount();
    expect(plot.destroy).toHaveBeenCalledTimes(2);
  });

  it('configures heatmap plots without a legend', () => {
    const { container } = render(<Graph {...defaultProps} displayMode={GraphDisplayMode.Heatmap} />);

    const options = plotSpy.mock.calls[0][2] as jquery.flot.plotOptions;
    expect(options.series?.heatmap).toBe(true);
    expect(options.series?.lines).toEqual({ show: false });
    expect(container.querySelector('.graph-legend')).toBeNull();
  });
});
