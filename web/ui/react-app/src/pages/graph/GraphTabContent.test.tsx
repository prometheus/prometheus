import React from 'react';
import { render, screen } from '@testing-library/react';
import { GraphTabContent } from './GraphTabContent';
import { GraphDisplayMode } from './Panel';

const defaultProps = {
  exemplars: undefined,
  displayMode: GraphDisplayMode.Lines,
  useLocalTime: false,
  showExemplars: false,
  handleTimeRangeSelection: jest.fn(),
  lastQueryParams: null,
  id: 'panel-1',
};

describe('GraphTabContent', () => {
  it.each([
    { data: null, color: 'light', message: 'No data queried yet' },
    { data: { resultType: 'matrix', result: [] }, color: 'secondary', message: 'Empty query result' },
    {
      data: { resultType: 'vector', result: [{}] },
      color: 'danger',
      message: "Query result is of wrong type 'vector', should be 'matrix' (range vector).",
    },
  ])('renders the $color alert for unsupported data', ({ data, color, message }) => {
    render(<GraphTabContent {...defaultProps} data={data} />);

    const alert = screen.getByRole('alert');
    expect(alert.classList.contains(`alert-${color}`)).toBe(true);
    expect(alert.textContent).toBe(message);
  });
});
