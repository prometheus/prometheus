import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import { Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList } from './pages';
import PathPrefixProps from './types/PathPrefixProps';

interface AppProps {
  consolesLink: string | null;
}

const App: FC<PathPrefixProps & AppProps> = ({ pathPrefix, consolesLink }) => {
  return (
    <>
      <Navigation pathPrefix={pathPrefix} consolesLink={consolesLink} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/graph`} />

          {/*
            NOTE: Any route added here needs to also be added to the list of
            React-handled router paths ("reactRouterPaths") in /web/web.go.
          */}
          <PanelList path="/graph" pathPrefix={pathPrefix} />
          <Alerts path="/alerts" pathPrefix={pathPrefix} />
          <Config path="/config" pathPrefix={pathPrefix} />
          <Flags path="/flags" pathPrefix={pathPrefix} />
          <Rules path="/rules" pathPrefix={pathPrefix} />
          <ServiceDiscovery path="/service-discovery" pathPrefix={pathPrefix} />
          <Status path="/status" pathPrefix={pathPrefix} />
          <TSDBStatus path="/tsdb-status" pathPrefix={pathPrefix} />
          <Targets path="/targets" pathPrefix={pathPrefix} />
        </Router>
      </Container>
    </>
  );
};

export default App;
