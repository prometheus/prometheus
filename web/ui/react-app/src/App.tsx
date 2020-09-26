import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import { Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList } from './pages';

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
  if (basePath.length > 1) {
    for (let i = 0; i < paths.length; i++) {
      if (basePath.endsWith(paths[i])) {
        basePath = basePath.slice(0, basePath.length - paths[i].length);
        break;
      }
    }
  }

  // When the React version of the UI is relocated from /new to /,
  // remove the "../" prefix here and all of the things will still work.
  const apiPath = '../api/v1';

  return (
    <>
      <Navigation pathPrefix={basePath} consolesLink={consolesLink} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${basePath}`}>
          <Redirect from="/" to={`graph`} noThrow />

          {/*
            NOTE: Any route added here needs to also be added to the list of
            React-handled router paths ("reactRouterPaths") in /web/web.go.
          */}
          <PanelList path="/graph" pathPrefix={basePath} apiPath={apiPath} />
          <Alerts path="/alerts" pathPrefix={basePath} apiPath={apiPath} />
          <Config path="/config" pathPrefix={basePath} apiPath={apiPath} />
          <Flags path="/flags" pathPrefix={basePath} apiPath={apiPath} />
          <Rules path="/rules" pathPrefix={basePath} apiPath={apiPath} />
          <ServiceDiscovery path="/service-discovery" pathPrefix={basePath} apiPath={apiPath} />
          <Status path="/status" pathPrefix={basePath} apiPath={apiPath} />
          <TSDBStatus path="/tsdb-status" pathPrefix={basePath} apiPath={apiPath} />
          <Targets path="/targets" pathPrefix={basePath} apiPath={apiPath} />
        </Router>
      </Container>
    </>
  );
};

export default App;
