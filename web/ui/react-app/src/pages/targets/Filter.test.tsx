import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import Filter, { Expanded, FilterData } from './Filter';

const initialFilter: FilterData = {
  showHealthy: true,
  showUnhealthy: true,
};

const initialExpanded: Expanded = {
  scrapePool1: true,
  scrapePool2: true,
};

describe('Filter', () => {
  it('renders the active filter and reports filter changes', () => {
    const setFilter = jest.fn();
    const setExpanded = jest.fn();
    render(<Filter filter={initialFilter} setFilter={setFilter} expanded={initialExpanded} setExpanded={setExpanded} />);

    expect(screen.getByRole('button', { name: 'All' }).classList.contains('active')).toBe(true);
    expect(screen.getByRole('button', { name: 'Unhealthy' }).classList.contains('active')).toBe(false);

    fireEvent.click(screen.getByRole('button', { name: 'All' }));
    expect(setFilter).toHaveBeenLastCalledWith({ showHealthy: true, showUnhealthy: true });

    fireEvent.click(screen.getByRole('button', { name: 'Unhealthy' }));
    expect(setFilter).toHaveBeenLastCalledWith({ showHealthy: false, showUnhealthy: true });
  });

  it.each([
    {
      name: 'expanded to collapsed',
      initial: initialExpanded,
      button: 'Collapse All',
      expected: { scrapePool1: false, scrapePool2: false },
    },
    {
      name: 'collapsed to expanded',
      initial: { scrapePool1: false, scrapePool2: false },
      button: 'Expand All',
      expected: initialExpanded,
    },
    {
      name: 'partially expanded to expanded',
      initial: { scrapePool1: true, scrapePool2: false },
      button: 'Expand All',
      expected: initialExpanded,
    },
  ])('$name', ({ initial, button, expected }) => {
    const setExpanded = jest.fn();
    render(<Filter filter={initialFilter} setFilter={jest.fn()} expanded={initial} setExpanded={setExpanded} />);

    fireEvent.click(screen.getByRole('button', { name: button }));
    expect(setExpanded).toHaveBeenCalledWith(expected);
  });
});
