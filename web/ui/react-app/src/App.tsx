import React, { Component } from 'react';
import Navigation from "./Navbar";
import { Container } from 'reactstrap';

import './App.css';
import { Router } from '@reach/router';
import {
  Alerts,
  Config,
  Flags,
  Rules,
  Services,
  Status,
  Targets,
  PanelList,
} from './pages';

class App extends Component {
  render() {
    return (
      <>
        <Navigation />
        <Container fluid>
          <Router basepath="/react">
            <PanelList path="/graph" />
            <Alerts path="/alerts" />
            <Config path="/config" />
            <Flags path="/flags" />
            <Rules path="/rules" />
            <Services path="/service-discovery" />
            <Status path="/status" />
            <Targets path="/targets" />
          </Router>
        </Container>
      </>
    );
  }
}

export default App;
