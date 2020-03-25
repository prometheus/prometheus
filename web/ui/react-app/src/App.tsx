import React, { FC } from 'react';
import { Container } from 'reactstrap';
import { Router, Redirect } from '@reach/router';

import Navigation from './Navbar';
import { Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList } from './pages';
import useGlobalVars from './hooks/useGlobalVars';
import './App.css';

const App: FC = () => {
  const { pathPrefix } = useGlobalVars();
  return (
    <>
      <Navigation />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/graph`} />

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
    </>
  );
};

export default App;
