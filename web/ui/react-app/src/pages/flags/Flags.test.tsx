import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { FlagsContent } from './Flags';

const sampleFlagsResponse = {
  'alertmanager.notification-queue-capacity': '10000',
  'alertmanager.timeout': '10s',
  'config.file': './documentation/examples/prometheus.yml',
  'query.timeout': '2m',
  'web.user-assets': '',
};

const renderedRows = (container: HTMLElement): HTMLTableRowElement[] =>
  Array.from(container.querySelectorAll<HTMLTableRowElement>('tbody tr'));

describe('FlagsContent', () => {
  it('renders an empty table when data is missing', () => {
    const { container } = render(<FlagsContent />);

    expect(screen.getByRole('heading', { name: 'Command-Line Flags' })).toBeTruthy();
    expect(renderedRows(container)).toHaveLength(0);
  });

  it('sorts flags alphabetically by default and reverses the order', () => {
    const { container } = render(<FlagsContent data={sampleFlagsResponse} />);

    expect(renderedRows(container)[0].textContent).toContain('--alertmanager.notification-queue-capacity');

    fireEvent.click(screen.getByText('Flag'));
    expect(renderedRows(container)[0].textContent).toContain('--web.user-assets');
  });

  it.each([
    { search: 'timeout', expected: ['--alertmanager.timeout', '--query.timeout'] },
    { search: '10s', expected: ['--alertmanager.timeout'] },
  ])('filters flags by name or value: $search', ({ search, expected }) => {
    const { container } = render(<FlagsContent data={sampleFlagsResponse} />);

    fireEvent.change(screen.getByPlaceholderText('Filter by flag name or value...'), { target: { value: search } });

    const rows = renderedRows(container);
    expect(rows).toHaveLength(expected.length);
    expected.forEach((flag) => expect(rows.some((row) => row.textContent?.includes(flag))).toBe(true));
  });
});
