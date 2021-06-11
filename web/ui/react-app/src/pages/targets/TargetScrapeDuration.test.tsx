import * as React from 'react';
import { shallow } from 'enzyme';
import TargetScrapeDuration from './TargetScrapeDuration';
import { Tooltip } from 'reactstrap';
import toJson from 'enzyme-to-json';

describe('targetScrapeDuration', () => {
  const defaultProps = {
    duration: 0.008,
    labels: {
      __address__: 'localhost:9100',
      __metrics_path__: '/metrics',
      __scheme__: 'http',
      __scrape_interval__: '15s',
      __scrape_timeout: '10s',
      job: 'node_exporter',
    },
    idx: 1,
    scrapePool: 'cortex/node-exporter_group/0',
  };
  const targetScrapeDuration = shallow(<TargetScrapeDuration {...defaultProps} />);

  it('renders duration correctly', () => {
    const duration = targetScrapeDuration.find('.scrape-duration-container');
    expect(duration.text()).toBe('8.000ms');
  });

  it('renders a tooltip for scrape interval and timeout', () => {
    const tooltip = targetScrapeDuration.find(Tooltip);
    expect(tooltip).toHaveLength(1);
    expect(tooltip.prop('isOpen')).toBe(false);
    expect(tooltip.prop('target')).toEqual('scrape-duration-cortex\\/node-exporter_group\\/0-1');
  });

  it('renders scrape interval and timeout', () => {
    expect(toJson(targetScrapeDuration)).toMatchSnapshot();
  });
});
