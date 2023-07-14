import * as React from 'react';
import { shallow } from 'enzyme';
import { ExploreContent } from './Explore';
import { Input, Table } from 'reactstrap';
import toJson from 'enzyme-to-json';

const sampleMetadataResponse = {
    'prometheus_web_federation_warnings_total': [
      {
        'type': 'counter',
        'help': 'Total number of warnings that occurred while sending federation responses.',
        'unit': '',
      }
    ],
    'promttp_metric_handler_requests_in_flight': [
      {
        'type': 'gauge',
        'help': 'Current numver of scrapes being served.',
        'unit': '',
      }
    ]
};

describe('Explore', () => {
  it('renders a table with properly configured props', () => {
    const w = shallow(<ExploreContent data={sampleMetadataResponse} />);
    const table = w.find(Table);
    expect(table.props()).toMatchObject({
      bordered: true,
      size: 'sm',
      striped: true,
    });
  });

  it('should not fail if data is missing', () => {
    expect(shallow(<ExploreContent />)).toHaveLength(1);
  });

  it('should match snapshot', () => {
    const w = shallow(<ExploreContent data={sampleMetadataResponse} />);
    expect(toJson(w)).toMatchSnapshot();
  });

  it('is sorted by metric by default', (): void => {
    const w = shallow(<ExploreContent data={sampleMetadataResponse} />);
    const td = w.find('tbody').find('td').find('span').first();
    expect(td.html()).toBe('<span>prometheus_web_federation_warnings_total</span>');
  });

  it('sorts', (): void => {
    const w = shallow(<ExploreContent data={sampleMetadataResponse} />);
    const th = w
      .find('thead')
      .find('td')
      .filterWhere((td): boolean => td.hasClass('Metric'));
    th.simulate('click');
    const td = w.find('tbody').find('td').find('span').first();
    expect(td.html()).toBe('<span>promttp_metric_handler_requests_in_flight</span>');
  });

  it('filters by metric name', (): void => {
    const w = shallow(<ExploreContent data={sampleMetadataResponse} />);
    const input = w.find(Input);
    input.simulate('change', { target: { value: 'timeout' } });
    const tds = w
      .find('tbody')
      .find('td')
      .filterWhere((code) => code.hasClass('flag-item'));
    expect(tds.length).toEqual(1);
  });

});
