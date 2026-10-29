import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import DataTable, { DataTableProps } from './DataTable';

jest.mock('./SeriesName', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({ labels, format }: { labels: Record<string, string> | null; format: boolean }) =>
      React.createElement('span', { 'data-testid': 'series-name', 'data-formatted': format }, labels?.__name__ || 'scalar'),
  };
});

jest.mock('./HistogramChart', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({ scale }: { scale: string }) =>
      React.createElement('div', { 'data-testid': 'histogram-chart', 'data-scale': scale }),
  };
});

describe('DataTable', () => {
  it.each([
    { data: null, color: 'light', message: 'No data queried yet' },
    { data: { resultType: 'vector', result: [] }, color: 'secondary', message: 'Empty query result' },
  ])('renders an alert for unavailable data', ({ data, color, message }) => {
    render(<DataTable useLocalTime={false} data={data as DataTableProps['data']} />);

    const alert = screen.getByRole('alert');
    expect(alert.classList.contains(`alert-${color}`)).toBe(true);
    expect(alert.textContent).toBe(message);
  });

  it('renders vector values with series names', () => {
    const data: DataTableProps['data'] = {
      resultType: 'vector',
      result: [
        { metric: { __name__: 'metric_name_1', label: 'value' }, value: [1572098246.599, '0'] },
        { metric: { __name__: 'metric_name_2', label: 'value' }, value: [1572098246.599, '1'] },
      ],
    };

    render(<DataTable data={data} useLocalTime={false} />);

    expect(screen.getAllByTestId('series-name').map((series) => series.textContent)).toEqual([
      'metric_name_1',
      'metric_name_2',
    ]);
    expect(screen.getAllByRole('row').map((row) => row.textContent)).toEqual(['metric_name_10', 'metric_name_21']);
  });

  it('renders histogram values and switches their scale', () => {
    const data: DataTableProps['data'] = {
      resultType: 'vector',
      result: [
        {
          metric: { __name__: 'request_duration' },
          histogram: [
            1572098246.599,
            {
              count: '10',
              sum: '3.3',
              buckets: [
                [1, '-1', '-0.5', '2'],
                [3, '-0.5', '0.5', '3'],
              ],
            },
          ],
        },
      ],
    };

    render(<DataTable data={data} useLocalTime={false} />);

    expect(screen.getByTestId('histogram-chart').getAttribute('data-scale')).toBe('exponential');
    expect(screen.getByText('Total count:').parentElement?.textContent).toContain('10');
    expect(screen.getByText('Sum:').parentElement?.textContent).toContain('3.3');
    expect(screen.getByText('[-1 -> -0.5)')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Linear' }));
    expect(screen.getByTestId('histogram-chart').getAttribute('data-scale')).toBe('linear');
  });

  it('limits large vector results and disables expensive label formatting', () => {
    const data: DataTableProps['data'] = {
      resultType: 'vector',
      result: Array.from({ length: 10001 }, (_, i) => ({
        metric: { __name__: `metric_name_${i}` },
        value: [1572098246.599, `${i}`],
      })),
    };

    const { container } = render(<DataTable data={data} useLocalTime={false} />);

    expect(container.querySelectorAll('.data-table > tbody > tr')).toHaveLength(10000);
    expect(screen.getByText(/Fetched 10001 metrics/).closest('[role="alert"]')?.textContent).toContain(
      'only displaying first 10000'
    );
    expect(screen.getByText(/Showing more than 1000 series/).closest('[role="alert"]')?.textContent).toContain(
      'turning off label formatting'
    );
    expect(screen.getAllByTestId('series-name')[0].getAttribute('data-formatted')).toBe('false');
  });

  it('renders matrix values with timestamp details', () => {
    const data: DataTableProps['data'] = {
      resultType: 'matrix',
      result: [
        {
          metric: { __name__: 'requests_total' },
          values: [
            [1572097950.93, '9'],
            [1572097965.931, '10'],
          ],
        },
      ],
    };

    const { container } = render(<DataTable data={data} useLocalTime={false} />);

    expect(screen.getByRole('row').textContent).toContain('requests_total9 @1572097950.9310 @1572097965.931');
    expect(Array.from(container.querySelectorAll('span[title]')).map((span) => span.getAttribute('title'))).toEqual([
      '2019-10-26T13:52:30.930Z',
      '2019-10-26T13:52:45.931Z',
    ]);
  });

  it.each([
    { resultType: 'scalar' as const, value: '5' },
    { resultType: 'string' as const, value: 'test' },
  ])('renders $resultType results', ({ resultType, value }) => {
    render(<DataTable data={{ resultType, result: [1572098246.599, value] }} useLocalTime={false} />);

    expect(screen.getByRole('row').textContent).toBe(`${resultType}${value}`);
  });
});
