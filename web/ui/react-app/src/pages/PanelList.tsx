import React, { Component, ChangeEvent } from 'react';
import { RouteComponentProps } from '@reach/router';

import { Alert, Button, Col, Row } from 'reactstrap';

import Panel, { PanelOptions, PanelDefaultOptions } from '../Panel';
import { decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString } from '../utils/urlParams';
import Checkbox from '../Checkbox';
import PathPrefixProps from '../PathPrefixProps';
import { generateID } from '../utils/func';

export type MetricGroup = { title: string; items: string[] };
export type PanelMeta = { key: string; options: PanelOptions; id: string };

interface PanelListState {
  panels: PanelMeta[];
  pastQueries: string[];
  metricNames: string[];
  fetchMetricsError: string | null;
  timeDriftError: string | null;
}

const initialPanel = {
  id: generateID(),
  key: '0',
  options: PanelDefaultOptions,
};

class PanelList extends Component<RouteComponentProps & PathPrefixProps, PanelListState> {
  constructor(props: PathPrefixProps) {
    super(props);

    const urlPanels = decodePanelOptionsFromQueryString(window.location.search);

    this.state = {
      panels: urlPanels.length ? urlPanels : [initialPanel],
      pastQueries: [],
      metricNames: [],
      fetchMetricsError: null,
      timeDriftError: null,
    };
    !urlPanels.length && this.updateURL();
  }

  componentDidMount() {
    fetch(`${this.props.pathPrefix}/api/v1/label/__name__/values`, { cache: 'no-store' })
      .then(resp => {
        if (resp.ok) {
          return resp.json();
        } else {
          throw new Error('Unexpected response status when fetching metric names: ' + resp.statusText); // TODO extract error
        }
      })
      .then(json => {
        this.setState({ metricNames: json.data });
      })
      .catch(error => this.setState({ fetchMetricsError: error.message }));

    const browserTime = new Date().getTime() / 1000;
    fetch(`${this.props.pathPrefix}/api/v1/query?query=time()`, { cache: 'no-store' })
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
          throw new Error(
            'Detected ' +
              delta +
              ' seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.'
          );
        }
      })
      .catch(error => this.setState({ timeDriftError: error.message }));

    window.onpopstate = () => {
      const panels = decodePanelOptionsFromQueryString(window.location.search);
      if (panels.length) {
        this.setState({ panels });
      }
    };

    this.updatePastQueries();
  }

  isHistoryEnabled = () => JSON.parse(localStorage.getItem('enable-query-history') || 'false') as boolean;

  getHistoryItems = () => JSON.parse(localStorage.getItem('history') || '[]') as string[];

  toggleQueryHistory = (e: ChangeEvent<HTMLInputElement>) => {
    localStorage.setItem('enable-query-history', `${e.target.checked}`);
    this.updatePastQueries();
  };

  updatePastQueries = () => {
    this.setState({
      pastQueries: this.isHistoryEnabled() ? this.getHistoryItems() : [],
    });
  };

  handleQueryHistory = (query: string) => {
    const isSimpleMetric = this.state.metricNames.indexOf(query) !== -1;
    if (isSimpleMetric || !query.length) {
      return;
    }
    const historyItems = this.getHistoryItems();
    const extendedItems = historyItems.reduce(
      (acc, metric) => {
        return metric === query ? acc : [...acc, metric]; // Prevent adding query twice.
      },
      [query]
    );
    localStorage.setItem('history', JSON.stringify(extendedItems.slice(0, 50)));
    this.updatePastQueries();
  };

  handleOptionsChanged = (key: string, options: PanelOptions) => {
    const panels = this.state.panels.map(panel => {
      return key === panel.key
        ? {
            ...panel,
            options,
          }
        : panel;
    });
    this.setState({ panels }, this.updateURL);
  };

  updateURL() {
    const query = encodePanelOptionsToQueryString(this.state.panels);
    window.history.pushState({}, '', query);
  };

  addPanel = () => {
    const { panels } = this.state;
    const addedPanel = {
      id: generateID(),
      key: `${panels.length}`,
      options: PanelDefaultOptions,
    };
    this.setState({ panels: [...panels, addedPanel] }, this.updateURL);
  };

  removePanel = (key: string) => {
    let newKey = 0;
    const panels = this.state.panels.reduce<PanelMeta[]>((acc, panel) => {
      return panel.key !== key ? [...acc, { ...panel, key: `${newKey++}` }] : acc;
    }, []);
    this.setState({ panels }, this.updateURL);
  };

  render() {
    const { metricNames, pastQueries, timeDriftError, fetchMetricsError, panels } = this.state;
    const { pathPrefix } = this.props;
    return (
      <>
        <Row className="mb-2">
          <Checkbox
            id="query-history-checkbox"
            wrapperStyles={{ margin: '0 0 0 15px', alignSelf: 'center' }}
            onChange={this.toggleQueryHistory}
            defaultChecked={this.isHistoryEnabled()}
          >
            Enable query history
          </Checkbox>
        </Row>
        <Row>
          <Col>
            {timeDriftError && (
              <Alert color="danger">
                <strong>Warning:</strong> Error fetching server time: {timeDriftError}
              </Alert>
            )}
          </Col>
        </Row>
        <Row>
          <Col>
            {fetchMetricsError && (
              <Alert color="danger">
                <strong>Warning:</strong> Error fetching metrics list: {fetchMetricsError}
              </Alert>
            )}
          </Col>
        </Row>
        {panels.map(({ id, options, key }) => (
          <Panel
            onExecuteQuery={this.handleQueryHistory}
            key={id}
            options={options}
            onOptionsChanged={opts => this.handleOptionsChanged(key, opts)}
            removePanel={() => this.removePanel(key)}
            metricNames={metricNames}
            pastQueries={pastQueries}
            pathPrefix={pathPrefix}
          />
        ))}
        <Button color="primary" className="add-panel-btn" onClick={this.addPanel}>
          Add Panel
        </Button>
      </>
    );
  }
}

export default PanelList;
