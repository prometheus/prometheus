import * as React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import TargetScrapeDuration from './TargetScrapeDuration';

describe('TargetScrapeDuration', () => {
  it('opens the scrape duration tooltip', async () => {
    render(<TargetScrapeDuration duration={0.25} interval="15s" timeout="10s" idx={2} scrapePool="prometheus/test" />);

    const target = document.getElementById('scrape-duration-prometheus/test-2');
    expect(target).not.toBeNull();

    fireEvent.mouseOver(target!);

    expect(await screen.findByText('Interval: 15s')).toBeTruthy();
    expect(screen.getByText('Timeout: 10s')).toBeTruthy();
    expect(document.querySelector('.tooltip.show')).not.toBeNull();
  });
});
