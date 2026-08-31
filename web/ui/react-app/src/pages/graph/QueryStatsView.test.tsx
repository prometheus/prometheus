import React from 'react';
import { render, screen } from '@testing-library/react';
import QueryStatsView from './QueryStatsView';

describe('QueryStatsView', () => {
  it('renders query statistics', () => {
    const { container } = render(<QueryStatsView loadTime={100} resolution={5} resultSeries={10000} />);

    expect(container.querySelector('.query-stats')).toBeTruthy();
    expect(screen.getByText(/Load time: 100ms/).textContent).toBe(
      'Load time: 100ms   Resolution: 5s   Result series: 10000'
    );
  });
});
