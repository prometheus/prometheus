import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { sampleApiResponse } from './__testdata__/testdata';
import ScrapePoolList from './ScrapePoolList';
import { PathPrefixContext } from '../../contexts/PathPrefixContext';

const scrapePools = ['blackbox', 'node_exporter', 'prometheus/test'];

const renderList = (onPoolSelect = jest.fn()) =>
  render(
    <PathPrefixContext.Provider value="/path/prefix">
      <ScrapePoolList scrapePools={scrapePools} selectedPool={null} onPoolSelect={onPoolSelect} />
    </PathPrefixContext.Provider>
  );

describe('ScrapePoolList', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
    localStorage.clear();
    window.history.replaceState({}, '', '/targets');
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('fetches and renders active targets grouped by scrape pool', async () => {
    fetchMock.mockResponse(JSON.stringify(sampleApiResponse));

    renderList();

    expect(await screen.findByRole('link', { name: 'blackbox (3/3 up)' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'node_exporter (1/1 up)' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'prometheus/test (1/1 up)' })).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledWith('/path/prefix/api/v1/targets?state=active', {
      cache: 'no-store',
      credentials: 'same-origin',
    });

    fireEvent.change(screen.getByPlaceholderText('Filter by endpoint or labels'), { target: { value: 'node_exporter' } });
    await waitFor(() => expect(screen.queryByRole('link', { name: 'blackbox (3/3 up)' })).toBeNull());
    expect(screen.getByRole('link', { name: 'node_exporter (1/1 up)' })).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'show less' }));
    expect(screen.getByRole('button', { name: 'show more' })).toBeTruthy();

    const healthy = screen.getByRole('checkbox', { name: 'healthy' }) as HTMLInputElement;
    fireEvent.click(healthy);
    expect(healthy.checked).toBe(false);
  });

  it('opens the scrape-pool dropdown and selects a pool', async () => {
    fetchMock.mockResponse(JSON.stringify(sampleApiResponse));
    const onPoolSelect = jest.fn();
    renderList(onPoolSelect);
    await screen.findByRole('link', { name: 'blackbox (3/3 up)' });

    fireEvent.click(screen.getByRole('button', { name: 'All scrape pools' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'node_exporter' }));

    expect(onPoolSelect).toHaveBeenCalledWith('node_exporter');
  });

  it('displays fetch errors', async () => {
    fetchMock.mockReject(new Error('Error fetching targets'));

    renderList();

    expect((await screen.findByRole('alert')).textContent).toContain('Error fetching targets');
  });
});
