import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import Targets from './Targets';
import { scrapePoolsSampleAPI } from './__testdata__/testdata';

jest.mock('./ScrapePoolList', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({
      scrapePools,
      selectedPool,
      onPoolSelect,
    }: {
      scrapePools: string[];
      selectedPool: string | null;
      onPoolSelect: (name: string) => void;
    }) =>
      React.createElement(
        'div',
        { 'data-testid': 'scrape-pool-list', 'data-selected-pool': selectedPool },
        React.createElement('span', null, scrapePools.join(',')),
        React.createElement('button', { type: 'button', onClick: () => onPoolSelect('blackbox') }, 'Select blackbox')
      ),
  };
});

describe('Targets', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
    window.history.replaceState({}, '', '/targets?scrapePool=initial');
  });

  it('fetches scrape pools and passes selection changes to the list', async () => {
    fetchMock.mockResponseOnce(JSON.stringify(scrapePoolsSampleAPI));

    render(<Targets />);

    expect(screen.getByRole('heading', { name: 'Targets' })).toBeTruthy();
    const list = await screen.findByTestId('scrape-pool-list');
    expect(list.textContent).toContain('blackbox');
    expect(list.getAttribute('data-selected-pool')).toBe('initial');
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/scrape_pools', {
      cache: 'no-store',
      credentials: 'same-origin',
    });

    fireEvent.click(screen.getByRole('button', { name: 'Select blackbox' }));
    expect(screen.getByTestId('scrape-pool-list').getAttribute('data-selected-pool')).toBe('blackbox');
    expect(window.location.search).toBe('?scrapePool=blackbox');
  });

  it('starts with all scrape pools selected when the URL has no selection', async () => {
    window.history.replaceState({}, '', '/targets');
    fetchMock.mockResponseOnce(JSON.stringify(scrapePoolsSampleAPI));

    render(<Targets />);

    expect((await screen.findByTestId('scrape-pool-list')).getAttribute('data-selected-pool')).toBeNull();
  });
});
