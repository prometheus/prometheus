import React from 'react';
import { render, screen } from '@testing-library/react';
import SeriesName from './SeriesName';

const labels = {
  __name__: 'metric_name',
  label1: 'value_1',
  label2: 'value_2',
  label3: 'value_3',
};

describe('SeriesName', () => {
  it('renders scalar results', () => {
    render(<SeriesName labels={null} format={false} />);

    expect(screen.getByText('scalar')).toBeTruthy();
  });

  it('renders an unformatted series name', () => {
    render(<SeriesName labels={labels} format={false} />);

    expect(screen.getByText('metric_name{label1="value_1", label2="value_2", label3="value_3"}')).toBeTruthy();
  });

  it('renders formatted metric and label parts', () => {
    const { container } = render(<SeriesName labels={labels} format />);

    expect(container.querySelector('.legend-metric-name')?.textContent).toBe('metric_name');
    expect(Array.from(container.querySelectorAll('.legend-label-brace')).map((node) => node.textContent)).toEqual([
      '{',
      '}',
    ]);
    expect(Array.from(container.querySelectorAll('.legend-label-name')).map((node) => node.textContent)).toEqual([
      'label1',
      'label2',
      'label3',
    ]);
    expect(Array.from(container.querySelectorAll('.legend-label-value')).map((node) => node.textContent)).toEqual([
      '"value_1"',
      '"value_2"',
      '"value_3"',
    ]);
  });
});
