import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import { Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList } from './pages';
import { PathPrefixContext } from './contexts/PathPrefixContext';

interface AppProps {
  consolesLink: string | null;
}

const App: FC<AppProps> = ({ consolesLink }) => {
  // This dynamically/generically determines the pathPrefix by stripping the first known
  // endpoint suffix from the window location path. It works out of the box for both direct
  // hosting and reverse proxy deployments with no additional configurations required.
  let basePath = window.location.pathname;
  const paths = [
    '/graph',
    '/alerts',
    '/status',
    '/tsdb-status',
    '/flags',
    '/config',
    '/rules',
    '/targets',
    '/service-discovery',
  ];
  if (basePath.endsWith('/')) {
    basePath = basePath.slice(0, -1);
  }
  if (basePath.length > 1) {
    for (let i = 0; i < paths.length; i++) {
      if (basePath.endsWith(paths[i])) {
        basePath = basePath.slice(0, basePath.length - paths[i].length);
        break;
      }
    }
  }

  return (
    <PathPrefixContext.Provider value={basePath}>
      <Navigation consolesLink={consolesLink} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${basePath}`}>
          <Redirect from="/" to={`graph`} noThrow />
          {/*
              NOTE: Any route added here needs to also be added to the list of
              React-handled router paths ("reactRouterPaths") in /web/web.go.
            */}
          <PanelList path="/graph" />
          <Alerts path="/alerts" />
          <Config path="/config" />
          <Flags path="/flags" />
          <Rules path="/rules" />
          <ServiceDiscovery path="/service-discovery" />
          <Status path="/status" />
          <TSDBStatus path="/tsdb-status" />
          <Targets path="/targets" />
        </Router>
      </Container>
    </PathPrefixContext.Provider>
  );
};

export default App;
