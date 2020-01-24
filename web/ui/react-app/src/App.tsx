import React, { FC, useState } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import { Alerts, Config, Flags, Rules, Services, Status, Targets, TSDBStatus, PanelList } from './pages';
import PathPrefixProps from './types/PathPrefixProps';

const App: FC<PathPrefixProps> = ({ pathPrefix }) => {
  const [extraNavItem, setExtraNavItem] = useState<React.ReactNode>(null);

  return (
    <>
      <Navigation pathPrefix={pathPrefix} extraNavItem={extraNavItem} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/graph`} />

          {/*
            NOTE: Any route added here needs to also be added to the list of
            React-handled router paths ("reactRouterPaths") in /web/web.go.
          */}
          <PanelList path="/graph" pathPrefix={pathPrefix} setExtraNavItem={setExtraNavItem} />
          <Alerts path="/alerts" pathPrefix={pathPrefix} />
          <Config path="/config" pathPrefix={pathPrefix} />
          <Flags path="/flags" pathPrefix={pathPrefix} />
          <Rules path="/rules" pathPrefix={pathPrefix} />
          <Services path="/service-discovery" pathPrefix={pathPrefix} />
          <Status path="/status" pathPrefix={pathPrefix} />
          <TSDBStatus path="/tsdb-status" pathPrefix={pathPrefix} />
          <Targets path="/targets" pathPrefix={pathPrefix} />
        </Router>
      </Container>
    </>
  );
};

export default App;
