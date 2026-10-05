import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import TargetLabels from './TargetLabels';

describe('TargetLabels', () => {
  const defaultProps = {
    discoveredLabels: {
      __address__: 'localhost:9100',
      __metrics_path__: '/metrics',
      __scheme__: 'http',
      job: 'node_exporter',
    },
    labels: {
      instance: 'localhost:9100',
      job: 'node_exporter',
      foo: 'bar',
    },
  };

  it('renders target labels', () => {
    render(<TargetLabels {...defaultProps} />);

    expect(screen.getByText('instance="localhost:9100"')).toBeTruthy();
    expect(screen.getByText('job="node_exporter"')).toBeTruthy();
    expect(screen.getByText('foo="bar"')).toBeTruthy();
    expect(screen.queryByText('Discovered labels:')).toBeNull();
  });

  it('toggles discovered labels', () => {
    render(<TargetLabels {...defaultProps} />);

    fireEvent.click(screen.getByTitle('Show discovered (pre-relabeling) labels'));
    expect(screen.getByText('Discovered labels:')).toBeTruthy();
    expect(screen.getByText('__address__="localhost:9100"')).toBeTruthy();

    fireEvent.click(screen.getByTitle('Hide discovered (pre-relabeling) labels'));
    expect(screen.queryByText('Discovered labels:')).toBeNull();
  });
});
