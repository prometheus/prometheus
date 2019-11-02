/* eslint @typescript-eslint/camelcase: 0 */

import * as React from 'react';
import { shallow } from 'enzyme';
import SeriesLabels from './SeriesLabels';
import { Tooltip } from 'reactstrap';
import SeriesLabel from './SeriesLabel';
import toJson from 'enzyme-to-json';

describe('SeriesLabels', () => {
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
  const seriesLabels = shallow(<SeriesLabels {...defaultProps} />);

  it('renders a div of series labels', () => {
    const div = seriesLabels.find('div').filterWhere(elem => elem.hasClass('series-labels-container'));
    expect(div).toHaveLength(1);
    expect(div.prop('id')).toEqual('series-labels');
  });

  it('wraps each label in a SeriesLabel', () => {
    const l: { [key: string]: string } = defaultProps.labels;
    Object.keys(l).forEach((key: string): void => {
      const label = seriesLabels.find(SeriesLabel).filterWhere(label => label.prop('labelKey') === key);
      expect(label.prop('labelValue')).toEqual(l[key]);
    });
    expect(seriesLabels.find(SeriesLabel)).toHaveLength(3);
  });

  it('renders a tooltip for discovered labels', () => {
    const tooltip = seriesLabels.find(Tooltip);
    expect(tooltip).toHaveLength(1);
    expect(tooltip.prop('isOpen')).toBe(false);
    expect(tooltip.prop('target')).toBe('series-labels');
    expect(tooltip.prop('isOpen')).toBe(false);
  });

  it('renders discovered labels', () => {
    expect(toJson(seriesLabels)).toMatchSnapshot();
  });
});
