import React, { Component } from 'react';

import { Alert, Button, Col, Row } from 'reactstrap';

import Panel from './Panel';

class PanelList extends Component {
  constructor(props) {
    super(props);
    this.state = {
      panels: [],
      metrics: [],
      fetchMetricsError: null,
      timeDriftError: null,
    };
    this.key = 0;

    this.addPanel = this.addPanel.bind(this);
    this.removePanel = this.removePanel.bind(this);
  }

  componentDidMount() {
    this.addPanel();

    fetch("http://demo.robustperception.io:9090/api/v1/label/__name__/values", {cache: "no-store"})
    .then(resp => {
      if (resp.ok) {
        return resp.json();
      } else {
        throw new Error('Unexpected response status when fetching metric names: ' + resp.statusText); // TODO extract error
      }
    })
    .then(json => this.setState({ metrics: json.data }))
    .catch(error => this.setState({fetchMetricsError: error.message}));

    const browserTime = new Date().getTime() / 1000;
    fetch("http://demo.robustperception.io:9090/api/v1/query?query=time()", {cache: "no-store"})
    .then(resp => {
      if (resp.ok) {
        return resp.json();
      } else {
        throw new Error('Unexpected response status when fetching metric names: ' + resp.statusText); // TODO extract error
      }
    })
    .then(json => {
      const serverTime = json.data.result[0];
      const delta = Math.abs(browserTime - serverTime);

      if (delta >= 30) {
        throw new Error('Detected ' + delta + ' seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.');
      }
    })
    .catch(error => this.setState({timeDriftError: error.message}));
  }

  getKey() {
    return (this.key++).toString();
  }

  addPanel() {
    const panels = this.state.panels.slice();
    const key = this.getKey();
    panels.push({key: key});
    this.setState({panels: panels});
  }

  removePanel(key) {
    const panels = this.state.panels.filter(panel => {
      return panel.key !== key;
    });
    this.setState({panels: panels});
  }

  render() {
    return (
      <>
        <Row>
          <Col>
            {this.state.timeDriftError && <Alert color="danger"><strong>Warning:</strong> {this.state.timeDriftError}</Alert>}
          </Col>
        </Row>
        <Row>
          <Col>
            {this.state.fetchMetricsError && <Alert color="danger"><strong>Warning:</strong> Error fetching metrics list: {this.state.fetchMetricsError}</Alert>}
          </Col>
        </Row>
        {this.state.panels.map(p =>
          <Panel key={p.key} removePanel={() => this.removePanel(p.key)} metrics={this.state.metrics}/>
        )}
        <Button color="primary" className="add-panel-btn" onClick={this.addPanel}>Add Panel</Button>
      </>
    );
  }
}

export default PanelList;
