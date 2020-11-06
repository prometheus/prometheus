import React from 'react';
import { shallow } from 'enzyme';
import Targets from './Targets';
import Filter from './Filter';
import ScrapePoolList from './ScrapePoolList';

describe('Targets', () => {
  const defaultProps = {
    pathPrefix: '..',
  };
  const targets = shallow(<Targets {...defaultProps} />);
  describe('Header', () => {
    const h2 = targets.find('h2');
    it('renders a header', () => {
      expect(h2.text()).toEqual('Targets');
    });
    it('renders exactly one header', () => {
      const h2 = targets.find('h2');
      expect(h2).toHaveLength(1);
    });
  });
  it('renders a filter', () => {
    const filter = targets.find(Filter);
    expect(filter).toHaveLength(1);
    expect(filter.prop('filter')).toEqual({ showHealthy: true, showUnhealthy: true });
  });
  it('renders a scrape pool list', () => {
    const scrapePoolList = targets.find(ScrapePoolList);
    expect(scrapePoolList).toHaveLength(1);
    expect(scrapePoolList.prop('filter')).toEqual({ showHealthy: true, showUnhealthy: true });
  });
});
