import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import MetricsExplorer from './MetricsExplorer';

const metrics = ['go_test_1', 'prometheus_test_1'];
const getMetric = (name: string): HTMLElement =>
  screen.getByText((_, element) => element?.classList.contains('metric') === true && element.textContent === name);

describe('MetricsExplorer', () => {
  it('lists and filters metrics', () => {
    render(<MetricsExplorer show updateShow={jest.fn()} metrics={metrics} insertAtCursor={jest.fn()} />);

    expect(screen.getByRole('heading', { name: 'Metrics Explorer' })).toBeTruthy();
    expect(getMetric('go_test_1')).toBeTruthy();
    expect(getMetric('prometheus_test_1')).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText('Search'), { target: { value: 'go' } });
    expect(getMetric('go_test_1')).toBeTruthy();
    expect(screen.queryByText('prometheus_test_1')).toBeNull();
  });

  it('inserts the selected metric and closes the explorer', () => {
    const insertAtCursor = jest.fn();
    const updateShow = jest.fn();
    render(<MetricsExplorer show updateShow={updateShow} metrics={metrics} insertAtCursor={insertAtCursor} />);

    fireEvent.click(getMetric('go_test_1'));
    expect(insertAtCursor).toHaveBeenCalledWith('go_test_1');
    expect(updateShow).toHaveBeenCalledWith(false);
  });
});
