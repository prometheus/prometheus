import * as React from 'react';
import { shallow } from 'enzyme';
import SeriesLabel from './SeriesLabel';
import { Badge } from 'reactstrap';

describe('SeriesLabel', () => {
  const defaultProps = {
    labelKey: 'job',
    labelValue: 'prometheus',
  };
  const seriesLabel = shallow(<SeriesLabel {...defaultProps} />);

  it('renders a badge', () => {
    const badge = seriesLabel.find(Badge);
    expect(badge.prop('color')).toEqual('primary');
    expect(badge.prop('className')).toEqual('series-label');
    expect(badge.children().text()).toEqual('job="prometheus"');
  });
});
