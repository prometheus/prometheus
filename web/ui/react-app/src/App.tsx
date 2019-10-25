import React, { Component } from 'react';

import {
  Container,
} from 'reactstrap';

import './App.css';
import PanelList from './PanelList';

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
