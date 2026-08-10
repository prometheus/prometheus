import * as React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';

jest.mock('./Navbar', () => {
  const React = require('react');
  return () => React.createElement('nav', { 'data-testid': 'navigation' });
});

jest.mock('./pages', () => {
  const React = require('react');
  const page = (testID: string) => () => React.createElement('div', { 'data-testid': testID });

  return {
    AgentPage: page('agent-page'),
    AlertsPage: page('alerts-page'),
    ConfigPage: page('config-page'),
    FlagsPage: page('flags-page'),
    PanelListPage: page('graph-page'),
    RulesPage: page('rules-page'),
    ServiceDiscoveryPage: page('service-discovery-page'),
    StatusPage: page('status-page'),
    TargetsPage: page('targets-page'),
    TSDBStatusPage: page('tsdb-status-page'),
  };
});

const renderApp = (path: string, agentMode = false): ReturnType<typeof render> => {
  window.history.pushState({}, '', path);
  return render(<App consolesLink={null} agentMode={agentMode} ready={false} />);
};

describe('App', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it.each([
    ['/agent', 'agent-page'],
    ['/graph', 'graph-page'],
    ['/alerts', 'alerts-page'],
    ['/config', 'config-page'],
    ['/flags', 'flags-page'],
    ['/rules', 'rules-page'],
    ['/service-discovery', 'service-discovery-page'],
    ['/status', 'status-page'],
    ['/targets', 'targets-page'],
    ['/tsdb-status', 'tsdb-status-page'],
  ])('renders the route at %s', (path, pageTestID) => {
    const { container } = renderApp(path);

    expect(screen.getByTestId(pageTestID)).toBeTruthy();
    expect(screen.getByTestId('navigation')).toBeTruthy();
    expect(container.querySelector('.container-fluid')).not.toBeNull();
  });

  it.each([
    ['/', false, '/graph', 'graph-page'],
    ['/', true, '/agent', 'agent-page'],
    ['/prometheus', false, '/prometheus/graph', 'graph-page'],
    ['/prometheus', true, '/prometheus/agent', 'agent-page'],
  ])('redirects %s in agent mode %s to %s', async (path, agentMode, expectedPath, pageTestID) => {
    renderApp(path, agentMode);

    await waitFor(() => expect(window.location.pathname).toBe(expectedPath));
    expect(screen.getByTestId(pageTestID)).toBeTruthy();
  });

  it.each([
    ['/prometheus/alerts', 'alerts-page'],
    ['/prometheus/targets/', 'targets-page'],
  ])('renders prefixed route %s', (path, pageTestID) => {
    renderApp(path);

    expect(screen.getByTestId(pageTestID)).toBeTruthy();
  });
});
