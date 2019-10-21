import React, { Component } from 'react';

import {
  Button,
  Container,
  Col,
  Row,
} from 'reactstrap';

import PanelList from './PanelList';

import './App.css';

class App extends Component {
  render() {
    return (
      <Container fluid={true}>
        <Row>
          <Col>
            <Button
              className="float-right classic-ui-btn"
              color="link"
              onClick={() => {window.location.pathname = "../../graph"}}
              size="sm">
                Return to classic UI
            </Button>
          </Col>
        </Row>
        <PanelList />
      </Container>
    );
  }
}

export default App;
