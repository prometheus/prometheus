import React, { Component } from 'react';

import {
  Button,
  Container,
  Col,
  Row,
} from 'reactstrap';

import './App.css';
import PanelList from './PanelList';
import { library } from '@fortawesome/fontawesome-svg-core';
import { faSearch, faSpinner, faCheck } from '@fortawesome/free-solid-svg-icons';
import { faSquare, faCheckSquare } from '@fortawesome/free-regular-svg-icons';

library.add(
  faSearch,
  faSpinner,
  faCheck,
  faSquare,
  faCheckSquare
);

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
