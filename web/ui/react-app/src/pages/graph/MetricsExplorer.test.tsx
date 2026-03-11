import * as React from 'react';
import { mount, ReactWrapper } from 'enzyme';
import MetricsExplorer from './MetricsExplorer';
import { Input } from 'reactstrap';

describe('MetricsExplorer', () => {
  const spyInsertAtCursor = jest.fn().mockImplementation((value: string) => {
    value = value;
  });
  const metricsExplorerProps = {
    show: true,
    updateShow: (show: boolean): void => {
      show = show;
    },
    metrics: ['go_test_1', 'prometheus_test_1'],
    insertAtCursor: spyInsertAtCursor,
  };

  let metricsExplorer: ReactWrapper;
  beforeEach(() => {
    metricsExplorer = mount(<MetricsExplorer {...metricsExplorerProps} />);
  });

  it('renders an Input[type=text]', () => {
    const input = metricsExplorer.find(Input);
    expect(input.prop('type')).toEqual('text');
  });

  it('lists all metrics in props', () => {
    const metrics = metricsExplorer.find('.metric');
    expect(metrics).toHaveLength(metricsExplorerProps.metrics.length);
  });

  it('filters metrics with search', () => {
    const input = metricsExplorer.find(Input);
    input.simulate('change', { target: { value: 'go' } });
    const metrics = metricsExplorer.find('.metric');
    expect(metrics).toHaveLength(1);
  });

  it('handles click on metric', () => {
    const metric = metricsExplorer.find('.metric').at(0);
    metric.simulate('click');
    expect(metricsExplorerProps.insertAtCursor).toHaveBeenCalled();
  });
});
