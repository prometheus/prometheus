import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import AlertsContent from './AlertContents';

describe('AlertsContent', () => {
  const defaultProps = {
    groups: [],
    statsCount: {
      inactive: 3,
      pending: 2,
      firing: 1,
    },
  };

  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('renders alert-state counts and enabled filters', () => {
    render(<AlertsContent {...defaultProps} />);

    expect((screen.getByRole('checkbox', { name: 'inactive (3)' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'pending (2)' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'firing (1)' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'Show annotations' }) as HTMLInputElement).checked).toBe(false);
  });

  it.each(['inactive (3)', 'pending (2)', 'firing (1)'])('toggles the %s filter', (name) => {
    render(<AlertsContent {...defaultProps} />);
    const checkbox = screen.getByRole('checkbox', { name });

    fireEvent.click(checkbox);
    expect((checkbox as HTMLInputElement).checked).toBe(false);

    fireEvent.click(checkbox);
    expect((checkbox as HTMLInputElement).checked).toBe(true);
  });

  it('toggles annotations and persists the choice', () => {
    render(<AlertsContent {...defaultProps} />);
    const checkbox = screen.getByRole('checkbox', { name: 'Show annotations' });

    fireEvent.click(checkbox);
    expect((checkbox as HTMLInputElement).checked).toBe(true);
    expect(localStorage.getItem('alerts-annotations-status')).toBe('{"checked":true}');

    fireEvent.click(checkbox);
    expect((checkbox as HTMLInputElement).checked).toBe(false);
  });
});
