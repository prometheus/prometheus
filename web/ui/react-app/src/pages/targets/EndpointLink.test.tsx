import React from 'react';
import { shallow, mount } from 'enzyme';
import { Badge, Alert } from 'reactstrap';
import EndpointLink from './EndpointLink';

describe('EndpointLink', () => {
  it('renders a simple anchor if the endpoint has no query params', () => {
    const endpoint = 'http://100.104.208.71:15090/stats/prometheus';
    const globalURL = 'http://100.104.208.71:15090/stats/prometheus';
    const endpointLink = shallow(<EndpointLink endpoint={endpoint} globalUrl={globalURL} />);
    const anchor = endpointLink.find('a');
    expect(anchor.prop('href')).toEqual(globalURL);
    expect(anchor.children().text()).toEqual(endpoint);
    expect(endpointLink.find('br')).toHaveLength(0);
  });

  it('renders an anchor targeting endpoint but with query param labels if the endpoint has query params', () => {
    const endpoint = 'http://100.99.128.71:9115/probe?module=http_2xx&target=http://some-service';
    const globalURL = 'http://100.99.128.71:9115/probe?module=http_2xx&target=http://some-service';
    const endpointLink = shallow(<EndpointLink endpoint={endpoint} globalUrl={globalURL} />);
    const anchor = endpointLink.find('a');
    const badges = endpointLink.find(Badge);
    expect(anchor.prop('href')).toEqual(globalURL);
    expect(anchor.children().text()).toEqual('http://100.99.128.71:9115/probe');
    expect(endpointLink.find('br')).toHaveLength(1);
    expect(badges).toHaveLength(2);
    const moduleLabel = badges.filterWhere((badge) => badge.children().text() === 'module="http_2xx"');
    expect(moduleLabel.length).toEqual(1);
    const targetLabel = badges.filterWhere((badge) => badge.children().text() === 'target="http://some-service"');
    expect(targetLabel.length).toEqual(1);
  });

  it('renders an alert if url is invalid', () => {
    const endpointLink = shallow(<EndpointLink endpoint={'afdsacas'} globalUrl={'afdsacas'} />);
    const err = endpointLink.find(Alert);
    expect(err.render().text()).toEqual('Error: Invalid URL: afdsacas');
  });

  it('handles params with multiple values correctly', () => {
    const consoleSpy = jest.spyOn(console, 'warn');
    const endpoint = `http://example.com/federate?match[]={__name__="name1"}&match[]={__name__="name2"}&match[]={__name__="name3"}`;
    const globalURL = 'http://example.com/federate';
    const endpointLink = mount(<EndpointLink endpoint={endpoint} globalUrl={globalURL} />);
    const badges = endpointLink.find(Badge);
    expect(badges).toHaveLength(3);
    expect(consoleSpy).not.toHaveBeenCalled();
  });
});
