import React from 'react';
import { render, screen } from '@testing-library/react';
import { WALReplayData } from '../types/types';
import { StartingContent } from './withStartingIndicator';

describe('StartingContent', () => {
  it('does not show progress before WAL replay starts', () => {
    const status: WALReplayData = { min: 0, max: 0, current: 0 };

    render(<StartingContent status={status} isUnexpected={false} />);

    expect(screen.queryByRole('progressbar')).toBeNull();
  });

  it('renders WAL replay progress', () => {
    const status: WALReplayData = { min: 0, max: 20, current: 1 };

    render(<StartingContent status={status} isUnexpected={false} />);

    expect(screen.getByText('Replaying WAL (1/20)')).toBeTruthy();
    const progress = screen.getByRole('progressbar');
    expect(progress.getAttribute('aria-valuenow')).toBe('2');
    expect(progress.getAttribute('aria-valuemin')).toBe('0');
    expect(progress.getAttribute('aria-valuemax')).toBe('21');
  });

  it('marks completed WAL replay as successful', () => {
    const status: WALReplayData = { min: 0, max: 20, current: 20 };

    render(<StartingContent status={status} isUnexpected={false} />);

    const progress = screen.getByRole('progressbar');
    expect(progress.getAttribute('aria-valuenow')).toBe('21');
    expect(progress.classList.contains('bg-success')).toBe(true);
  });

  it('shows an error when startup fails unexpectedly', () => {
    render(<StartingContent isUnexpected />);

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain('Server is not responding');
    expect(alert.classList.contains('alert-danger')).toBe(true);
  });
});
