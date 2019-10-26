import React, { Component, ChangeEvent } from 'react';

import { Alert, Button, Col, Row } from 'reactstrap';

import Panel, { PanelOptions, PanelDefaultOptions } from './Panel';
import { decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString } from './utils/urlParams';
import Checkbox from './Checkbox';

export type MetricGroup = { title: string; items: string[] };

interface PanelListState {
  panels: {
    key: string;
    options: PanelOptions;
  }[];
  pastQueries: string[];
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
      pastQueries: [],
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
    .then(json => {
      this.setState({metricNames: json.data});
    })
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

    this.updatePastQueries();
  }

  isHistoryEnabled = () => JSON.parse(localStorage.getItem('enable-query-history') || 'false') as boolean;

  getHistoryItems = () => JSON.parse(localStorage.getItem('history') || '[]') as string[];

  toggleQueryHistory = (e: ChangeEvent<HTMLInputElement>) => {
    localStorage.setItem('enable-query-history', `${e.target.checked}`);
    this.updatePastQueries();
  }

  updatePastQueries = () => {
    this.setState({
      pastQueries: this.isHistoryEnabled() ? this.getHistoryItems() : []
    });
  }

  handleQueryHistory = (query: string) => {
    const isSimpleMetric = this.state.metricNames.indexOf(query) !== -1;
    if (isSimpleMetric || !query.length) {
      return;
    }
    const historyItems = this.getHistoryItems();
    const extendedItems = historyItems.reduce((acc, metric) => {
      return metric === query ? acc : [...acc, metric]; // Prevent adding query twice.
    }, [query]);
    localStorage.setItem('history', JSON.stringify(extendedItems.slice(0, 50)));
    this.updatePastQueries();
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
    this.setState({panels: newPanels}, this.updateURL)
  }

  updateURL(): void {
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
    const { metricNames, pastQueries, timeDriftError, fetchMetricsError } = this.state;
    return (
      <>
        <Row className="mb-2">
          <Checkbox
            id="query-history-checkbox"
            wrapperStyles={{ margin: '0 0 0 15px', alignSelf: 'center' }}
            onChange={this.toggleQueryHistory}
            defaultChecked={this.isHistoryEnabled()}>
            Enable query history
          </Checkbox>
          <Col>
            <Button
              className="float-right classic-ui-btn"
              color="link"
              onClick={() => { window.location.pathname = "../../graph" }}
              size="sm">
              Return to classic UI
            </Button>
          </Col>
        </Row>
        <Row>
          <Col>
            {timeDriftError && <Alert color="danger"><strong>Warning:</strong> Error fetching server time: {this.state.timeDriftError}</Alert>}
          </Col>
        </Row>
        <Row>
          <Col>
            {fetchMetricsError && <Alert color="danger"><strong>Warning:</strong> Error fetching metrics list: {this.state.fetchMetricsError}</Alert>}
          </Col>
        </Row>
        {this.state.panels.map(p =>
          <Panel
            onExecuteQuery={this.handleQueryHistory}
            key={p.key}
            options={p.options}
            onOptionsChanged={(opts: PanelOptions) => this.handleOptionsChanged(p.key, opts)}
            removePanel={() => this.removePanel(p.key)}
            metricNames={metricNames}
            pastQueries={pastQueries}
          />
        )}
        <Button color="primary" className="add-panel-btn" onClick={this.addPanel}>Add Panel</Button>
      </>
    );
  }
}

export default PanelList;
