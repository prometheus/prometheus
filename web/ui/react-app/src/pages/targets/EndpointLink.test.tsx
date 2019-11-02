import React from 'react';
import { shallow } from 'enzyme';
import EndpointLink from './EndpointLink';
import SeriesLabel from './SeriesLabel';
import ErrorAlert from '../../ErrorAlert';

describe('EndpointLink', () => {
  it('renders a simple anchor if the endpoint has no query params', () => {
    const endpoint = 'http://100.104.208.71:15090/stats/prometheus';
    const endpointLink = shallow(<EndpointLink endpoint={endpoint} />);
    const anchor = endpointLink.find('a');
    expect(anchor.prop('href')).toEqual(endpoint);
    expect(anchor.children().text()).toEqual(endpoint);
    expect(endpointLink.find('br')).toHaveLength(0);
  });
  it('renders an anchor targeting endpoint but with query param labels if the endpoint has query params', () => {
    const endpoint = 'http://100.99.128.71:9115/probe?module=http_2xx&target=http://some-service';
    const endpointLink = shallow(<EndpointLink endpoint={endpoint} />);
    const anchor = endpointLink.find('a');
    const labels = endpointLink.find(SeriesLabel);
    expect(anchor.prop('href')).toEqual(endpoint);
    expect(anchor.children().text()).toEqual('http://100.99.128.71:9115/probe');
    expect(endpointLink.find('br')).toHaveLength(1);
    expect(labels).toHaveLength(2);
    expect(labels.at(0).prop('labelKey')).toEqual('module');
    expect(labels.at(0).prop('labelValue')).toEqual('http_2xx');
    expect(labels.at(1).prop('labelKey')).toEqual('target');
    expect(labels.at(1).prop('labelValue')).toEqual('http://some-service');
  });
  it('renders and error alert if url is invalid', () => {
    const endpointLink = shallow(<EndpointLink endpoint={'afdsacas'} />);
    const err = endpointLink.find(ErrorAlert);
    expect(err.prop('summary')).toEqual('Invalid URL');
  });
});
