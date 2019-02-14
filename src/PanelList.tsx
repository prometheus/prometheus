import React, { Component } from 'react';

import { Alert, Button, Col, Row } from 'reactstrap';

import Panel from './Panel';

interface PanelListState {
  panels: {
    key: string,
  }[],
  metricNames: string[],
  fetchMetricsError: string | null,
  timeDriftError: string | null,
}

class PanelList extends Component<any, PanelListState> {
  private key: number;

  constructor(props: any) {
    super(props);

    this.state = {
      panels: [],
      metricNames: [],
      fetchMetricsError: null,
      timeDriftError: null,
    };

    this.key = 0;
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
    .then(json => this.setState({ metricNames: json.data }))
    .catch(error => this.setState({ fetchMetricsError: error.message }));

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
    .catch(error => this.setState({ timeDriftError: error.message }));
  }

  getKey(): string {
    return (this.key++).toString();
  }

  addPanel = (): void => {
    const panels = this.state.panels.slice();
    const key = this.getKey();
    panels.push({key: key});
    this.setState({panels: panels});
  }

  removePanel = (key: string): void => {
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
          <Panel key={p.key} removePanel={() => this.removePanel(p.key)} metricNames={this.state.metricNames}/>
        )}
        <Button color="primary" className="add-panel-btn" onClick={this.addPanel}>Add Panel</Button>
      </>
    );
  }
}

export default PanelList;
