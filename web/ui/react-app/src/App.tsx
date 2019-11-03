import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import { Alerts, Config, Flags, Rules, Services, Status, Targets, PanelList } from './pages';
import PathPrefixProps from './PathPrefixProps';

const App: FC<PathPrefixProps> = ({ pathPrefix }) => {
  return (
    <>
      <Navigation pathPrefix={pathPrefix} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/graph`} />

          <PanelList path="/graph" pathPrefix={pathPrefix} />
          <Alerts path="/alerts" pathPrefix={pathPrefix} />
          <Config path="/config" pathPrefix={pathPrefix} />
          <Flags path="/flags" pathPrefix={pathPrefix} />
          <Rules path="/rules" pathPrefix={pathPrefix} />
          <Services path="/service-discovery" pathPrefix={pathPrefix} />
          <Status path="/status" pathPrefix={pathPrefix} />
          <Targets path="/targets" pathPrefix={pathPrefix} />
        </Router>
      </Container>
    </>
  );
};

export default App;
