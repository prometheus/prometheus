import React, { Component } from 'react';

import { Container } from 'reactstrap';

import PanelList from './PanelList';

import './App.css';

class App extends Component {
  render() {
    return (
      <Container fluid={true}>
        <PanelList />
      </Container>
    );
  }
}

export default App;
