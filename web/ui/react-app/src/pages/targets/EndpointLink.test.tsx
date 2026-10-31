import React from 'react';
import { render, screen } from '@testing-library/react';
import EndpointLink from './EndpointLink';

describe('EndpointLink', () => {
  it('renders a simple anchor when the endpoint has no query parameters', () => {
    const endpoint = 'http://100.104.208.71:15090/stats/prometheus';

    render(<EndpointLink endpoint={endpoint} globalUrl={endpoint} />);

    expect(screen.getByRole('link', { name: endpoint }).getAttribute('href')).toBe(endpoint);
    expect(document.querySelector('br')).toBeNull();
  });

  it('renders query parameters as labels', () => {
    const endpoint = 'http://100.99.128.71:9115/probe?module=http_2xx&target=http://some-service';

    render(<EndpointLink endpoint={endpoint} globalUrl={endpoint} />);

    expect(screen.getByRole('link', { name: 'http://100.99.128.71:9115/probe' }).getAttribute('href')).toBe(endpoint);
    expect(screen.getByText('module="http_2xx"')).toBeTruthy();
    expect(screen.getByText('target="http://some-service"')).toBeTruthy();
  });

  // URL does not parse IPv6 zone IDs, so the component reconstructs the display URL.
  it('renders query parameters for an IPv6 endpoint with a zone ID', () => {
    const endpoint =
      'http://[fe80::f1ee:adeb:371d:983%eth1]:9100/stats/prometheus?module=http_2xx&target=http://some-service';

    render(<EndpointLink endpoint={endpoint} globalUrl={endpoint} />);

    expect(
      screen.getByRole('link', { name: 'http://[fe80::f1ee:adeb:371d:983%eth1]:9100/stats/prometheus' }).getAttribute('href')
    ).toBe(endpoint);
    expect(screen.getByText('module="http_2xx"')).toBeTruthy();
    expect(screen.getByText('target="http://some-service"')).toBeTruthy();
  });

  it('preserves repeated query parameters', () => {
    const endpoint = `http://example.com/federate?match[]={__name__="name1"}&match[]={__name__="name2"}&match[]={__name__="name3"}`;

    render(<EndpointLink endpoint={endpoint} globalUrl="http://example.com/federate" />);

    expect(screen.getAllByText(/^match\[\]=/).map((badge) => badge.textContent)).toEqual([
      'match[]="{__name__="name1"}"',
      'match[]="{__name__="name2"}"',
      'match[]="{__name__="name3"}"',
    ]);
  });
});
