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
  };
  const targetLabels = shallow(<TargetLabels {...defaultProps} />);

  it('renders a div of series labels', () => {
    const div = targetLabels.find('div').filterWhere((elem) => elem.hasClass('series-labels-container'));
    expect(div).toHaveLength(1);
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
});
