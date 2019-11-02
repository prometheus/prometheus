import React from 'react';
import { shallow } from 'enzyme';
import Targets from './Targets';
import Filter from './Filter';
import JobsList from './JobsList';

describe('Targets', () => {
  const defaultProps = {
    pathPrefix: '..',
  };
  const targets = shallow(<Targets {...defaultProps} />);
  it('renders a header', () => {
    const h1 = targets.find('h1');
    expect(h1).toHaveLength(1);
    expect(h1.text()).toEqual('Targets');
  });
  it('renders a filter', () => {
    const filter = targets.find(Filter);
    expect(filter).toHaveLength(1);
    expect(filter.prop('filter')).toEqual({ showHealthy: true, showUnhealthy: true });
  });
  it('renders a jobs list', () => {
    const jobsList = targets.find(JobsList);
    expect(jobsList).toHaveLength(1);
    expect(jobsList.prop('filter')).toEqual({ showHealthy: true, showUnhealthy: true });
    expect(jobsList.prop('pathPrefix')).toEqual(defaultProps.pathPrefix);
  });
});
