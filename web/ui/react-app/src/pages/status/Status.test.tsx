import React from 'react';
import { render, screen } from '@testing-library/react';
import { StatusContent } from './Status';

describe('StatusContent', () => {
  it('formats configured values, renders alertmanager links, and skips hidden fields', () => {
    const data = {
      startTime: '2019-10-30T22:03:23.247913868+02:00',
      CWD: '/srv/prometheus',
      reloadConfigSuccess: true,
      activeAlertmanagers: [
        { url: 'https://alertmanager-1.example.com/api/v1/alerts' },
        { url: 'https://alertmanager-2.example.com/api/v1/alerts' },
      ],
      droppedAlertmanagers: [{ url: 'https://dropped.example.com/api/v1/alerts' }],
      customField: 'custom value',
    } as unknown as Record<string, string>;

    render(<StatusContent data={data} title="Runtime Information" />);

    expect(screen.getByRole('heading', { name: 'Runtime Information' })).toBeTruthy();
    expect(screen.getByText('Start time').closest('tr')?.textContent).toContain('Wed, 30 Oct 2019 20:03:23 GMT');
    expect(screen.getByText('Working directory').closest('tr')?.textContent).toContain('/srv/prometheus');
    expect(screen.getByText('Configuration reload').closest('tr')?.textContent).toContain('Successful');
    expect(screen.getByText('customField').closest('tr')?.textContent).toContain('custom value');

    const firstAlertmanager = screen.getByRole('link', { name: 'https://alertmanager-1.example.com' });
    expect(firstAlertmanager.getAttribute('href')).toBe('https://alertmanager-1.example.com/api/v1/alerts');
    expect(firstAlertmanager.closest('tr')?.textContent).toContain('/api/v1/alerts');
    expect(screen.getByRole('link', { name: 'https://alertmanager-2.example.com' })).toBeTruthy();
    expect(screen.queryByText(/dropped\.example\.com/)).toBeNull();
  });
});
