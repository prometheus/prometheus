import React, { Component } from 'react';

import { Alert, Button, Col, Row } from 'reactstrap';

import Panel, { PanelOptions, PanelDefaultOptions } from './Panel';
import { decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString } from './utils/urlParams';

interface PanelListState {
  panels: {
    key: string;
    options: PanelOptions;
  }[],
  metricNames: string[];
  fetchMetricsError: string | null;
  timeDriftError: string | null;
}

class PanelList extends Component<any, PanelListState> {
  private key: number = 0;

  constructor(props: any) {
    super(props);

    const urlPanels = decodePanelOptionsFromQueryString(window.location.search);

    this.state = {
      panels: urlPanels.length !== 0 ? urlPanels : [
        {
          key: this.getKey(),
          options: PanelDefaultOptions,
        },
      ],
      metricNames: [],
      fetchMetricsError: null,
      timeDriftError: null,
    };
  }

  componentDidMount() {
    fetch("../../api/v1/label/__name__/values", {cache: "no-store"})
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
    fetch("../../api/v1/query?query=time()", {cache: "no-store"})
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

    window.onpopstate = () => {
      const panels = decodePanelOptionsFromQueryString(window.location.search);
      if (panels.length !== 0) {
        this.setState({panels: panels});
      }
    }
  }

  getKey(): string {
    return (this.key++).toString();
  }

  handleOptionsChanged(key: string, opts: PanelOptions): void {
    const newPanels = this.state.panels.map(p => {
      if (key === p.key) {
        return {
          key: key,
          options: opts,
        }
      }
      return p;
    });
    console.log("UPDATE OP", key, opts);
    this.setState({panels: newPanels}, this.updateURL)
  }

  updateURL(): void {
    console.log("UPDATE");
    const query = encodePanelOptionsToQueryString(this.state.panels);
    window.history.pushState({}, '', query);
  }

  addPanel = (): void => {
    const panels = this.state.panels.slice();
    panels.push({
      key: this.getKey(),
      options: PanelDefaultOptions,
    });
    this.setState({panels: panels}, this.updateURL);
  }

  removePanel = (key: string): void => {
    const panels = this.state.panels.filter(panel => {
      return panel.key !== key;
    });
    this.setState({panels: panels}, this.updateURL);
  }

  render() {
    return (
      <>
        <Row>
          <Col>
            {this.state.timeDriftError && <Alert color="danger"><strong>Warning:</strong> Error fetching server time: {this.state.timeDriftError}</Alert>}
          </Col>
        </Row>
        <Row>
          <Col>
            {this.state.fetchMetricsError && <Alert color="danger"><strong>Warning:</strong> Error fetching metrics list: {this.state.fetchMetricsError}</Alert>}
          </Col>
        </Row>
        {this.state.panels.map(p =>
          <Panel
            key={p.key}
            options={p.options}
            onOptionsChanged={(opts: PanelOptions) => this.handleOptionsChanged(p.key, opts)}
            removePanel={() => this.removePanel(p.key)}
            metricNames={this.state.metricNames}
          />
        )}
        <Button color="primary" className="add-panel-btn" onClick={this.addPanel}>Add Panel</Button>
      </>
    );
  }
}

export default PanelList;
