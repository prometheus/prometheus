import * as React from 'react';
import { shallow } from 'enzyme';
import TargetLabels from './TargetLabels';
import { Tooltip, Badge } from 'reactstrap';
import toJson from 'enzyme-to-json';

describe('targetLabels', () => {
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
    idx: 1,
    scrapePool: 'cortex/node-exporter_group/0',
  };
  const targetLabels = shallow(<TargetLabels {...defaultProps} />);

  it('renders a div of series labels', () => {
    const div = targetLabels.find('div').filterWhere((elem) => elem.hasClass('series-labels-container'));
    expect(div).toHaveLength(1);
    expect(div.prop('id')).toEqual('series-labels-cortex/node-exporter_group/0-1');
  });

  it('wraps each label in a label badge', () => {
    const l: { [key: string]: string } = defaultProps.labels;
    Object.keys(l).forEach((labelName: string): void => {
      const badge = targetLabels
        .find(Badge)
        .filterWhere((badge) => badge.children().text() === `${labelName}="${l[labelName]}"`);
      expect(badge).toHaveLength(1);
    });
    expect(targetLabels.find(Badge)).toHaveLength(3);
  });

  it('renders a tooltip for discovered labels', () => {
    const tooltip = targetLabels.find(Tooltip);
    expect(tooltip).toHaveLength(1);
    expect(tooltip.prop('isOpen')).toBe(false);
    expect(tooltip.prop('target')).toEqual('series-labels-cortex\\/node-exporter_group\\/0-1');
  });

  it('renders discovered labels', () => {
    expect(toJson(targetLabels)).toMatchSnapshot();
  });
});
