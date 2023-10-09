import React from 'react';
import { shallow, mount } from 'enzyme';
import { Badge } from 'reactstrap';
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
  // In cases of IPv6 addresses with a Zone ID, URL may not be parseable.
  // See https://github.com/prometheus/prometheus/issues/9760
  it('renders an anchor for IPv6 link with zone ID including labels for query params', () => {
    const endpoint =
      'http://[fe80::f1ee:adeb:371d:983%eth1]:9100/stats/prometheus?module=http_2xx&target=http://some-service';
    const globalURL =
      'http://[fe80::f1ee:adeb:371d:983%eth1]:9100/stats/prometheus?module=http_2xx&target=http://some-service';
    const endpointLink = shallow(<EndpointLink endpoint={endpoint} globalUrl={globalURL} />);
    const anchor = endpointLink.find('a');
    const badges = endpointLink.find(Badge);
    expect(anchor.prop('href')).toEqual(globalURL);
    expect(anchor.children().text()).toEqual('http://[fe80::f1ee:adeb:371d:983%eth1]:9100/stats/prometheus');
    expect(endpointLink.find('br')).toHaveLength(1);
    expect(badges).toHaveLength(2);
    const moduleLabel = badges.filterWhere((badge) => badge.children().text() === 'module="http_2xx"');
    expect(moduleLabel.length).toEqual(1);
    const targetLabel = badges.filterWhere((badge) => badge.children().text() === 'target="http://some-service"');
    expect(targetLabel.length).toEqual(1);
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
