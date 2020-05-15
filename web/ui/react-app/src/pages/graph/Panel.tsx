import React, { Component } from 'react';

import { Alert, Button, Col, Nav, NavItem, NavLink, Row, TabContent, TabPane } from 'reactstrap';

import moment from 'moment-timezone';

import ExpressionInput from './ExpressionInput';
import GraphControls from './GraphControls';
import { GraphTabContent } from './GraphTabContent';
import DataTable from './DataTable';
import TimeInput from './TimeInput';
import QueryStatsView, { QueryStats } from './QueryStatsView';
import PathPrefixProps from '../../types/PathPrefixProps';
import { QueryParams } from '../../types/types';

interface PanelProps {
  options: PanelOptions;
  onOptionsChanged: (opts: PanelOptions) => void;
  useLocalTime: boolean;
  pastQueries: string[];
  metricNames: string[];
  removePanel: () => void;
  onExecuteQuery: (query: string) => void;
}

interface PanelState {
  data: any; // TODO: Type data.
  lastQueryParams: QueryParams | null;
  loading: boolean;
  error: string | null;
  stats: QueryStats | null;
  exprInputValue: string;
}

export interface PanelOptions {
  expr: string;
  type: PanelType;
  range: number; // Range in seconds.
  endTime: number | null; // Timestamp in milliseconds.
  resolution: number | null; // Resolution in seconds.
  stacked: boolean;
}

export enum PanelType {
  Graph = 'graph',
  Table = 'table',
}

export const PanelDefaultOptions: PanelOptions = {
  type: PanelType.Table,
  expr: '',
  range: 3600,
  endTime: null,
  resolution: null,
  stacked: false,
};

class Panel extends Component<PanelProps & PathPrefixProps, PanelState> {
  private abortInFlightFetch: (() => void) | null = null;

  constructor(props: PanelProps) {
    super(props);

    this.state = {
      data: null,
      lastQueryParams: null,
      loading: false,
      error: null,
      stats: null,
      exprInputValue: props.options.expr,
    };
  }

  componentDidUpdate({ options: prevOpts }: PanelProps) {
    const { endTime, range, resolution, type } = this.props.options;
    if (
      prevOpts.endTime !== endTime ||
      prevOpts.range !== range ||
      prevOpts.resolution !== resolution ||
      prevOpts.type !== type
    ) {
      this.executeQuery();
    }
  }

  componentDidMount() {
    this.executeQuery();
  }

  executeQuery = (): void => {
    const { exprInputValue: expr } = this.state;
    const queryStart = Date.now();
    this.props.onExecuteQuery(expr);
    if (this.props.options.expr !== expr) {
      this.setOptions({ expr });
    }
    if (expr === '') {
      return;
    }

    if (this.abortInFlightFetch) {
      this.abortInFlightFetch();
      this.abortInFlightFetch = null;
    }

    const abortController = new AbortController();
    this.abortInFlightFetch = () => abortController.abort();
    this.setState({ loading: true });

    const endTime = this.getEndTime().valueOf() / 1000; // TODO: shouldn't valueof only work when it's a moment?
    const startTime = endTime - this.props.options.range;
    const resolution = this.props.options.resolution || Math.max(Math.floor(this.props.options.range / 250), 1);
    const params: URLSearchParams = new URLSearchParams({
      query: expr,
    });

    let path: string;
    switch (this.props.options.type) {
      case 'graph':
        path = '/api/v1/query_range';
        params.append('start', startTime.toString());
        params.append('end', endTime.toString());
        params.append('step', resolution.toString());
        // TODO path prefix here and elsewhere.
        break;
      case 'table':
        path = '/api/v1/query';
        params.append('time', endTime.toString());
        break;
      default:
        throw new Error('Invalid panel type "' + this.props.options.type + '"');
    }

    fetch(`${this.props.pathPrefix}${path}?${params}`, {
      cache: 'no-store',
      credentials: 'same-origin',
      signal: abortController.signal,
    })
      .then(resp => resp.json())
      .then(json => {
        if (json.status !== 'success') {
          throw new Error(json.error || 'invalid response JSON');
        }

        let resultSeries = 0;
        if (json.data) {
          const { resultType, result } = json.data;
          if (resultType === 'scalar') {
            resultSeries = 1;
          } else if (result && result.length > 0) {
            resultSeries = result.length;
          }
        }

        this.setState({
          error: null,
          data: json.data,
          lastQueryParams: {
            startTime,
            endTime,
            resolution,
          },
          stats: {
            loadTime: Date.now() - queryStart,
            resolution,
            resultSeries,
          },
          loading: false,
        });
        this.abortInFlightFetch = null;
      })
      .catch(error => {
        if (error.name === 'AbortError') {
          // Aborts are expected, don't show an error for them.
          return;
        }
        this.setState({
          error: 'Error executing query: ' + error.message,
          loading: false,
        });
      });
  };

  setOptions(opts: object): void {
    const newOpts = { ...this.props.options, ...opts };
    this.props.onOptionsChanged(newOpts);
  }

  handleExpressionChange = (expr: string): void => {
    this.setState({ exprInputValue: expr });
  };

  handleChangeRange = (range: number): void => {
    this.setOptions({ range: range });
  };

  getEndTime = (): number | moment.Moment => {
    if (this.props.options.endTime === null) {
      return moment();
    }
    return this.props.options.endTime;
  };

  handleChangeEndTime = (endTime: number | null) => {
    this.setOptions({ endTime: endTime });
  };

  handleChangeResolution = (resolution: number | null) => {
    this.setOptions({ resolution: resolution });
  };

  handleChangeType = (type: PanelType) => {
    if (this.props.options.type === type) {
      return;
    }

    this.setState({ data: null });
    this.setOptions({ type: type });
  };

  handleChangeStacking = (stacked: boolean) => {
    this.setOptions({ stacked: stacked });
  };

  render() {
    const { pastQueries, metricNames, options } = this.props;
    return (
      <div className="panel">
        <Row>
          <Col>
            <ExpressionInput
              value={this.state.exprInputValue}
              onExpressionChange={this.handleExpressionChange}
              executeQuery={this.executeQuery}
              loading={this.state.loading}
              autocompleteSections={{
                'Query History': pastQueries,
                'Metric Names': metricNames,
              }}
            />
          </Col>
        </Row>
        <Row>
          <Col>{this.state.error && <Alert color="danger">{this.state.error}</Alert>}</Col>
        </Row>
        <Row>
          <Col>
            <Nav tabs>
              <NavItem>
                <NavLink
                  className={options.type === 'table' ? 'active' : ''}
                  onClick={() => this.handleChangeType(PanelType.Table)}
                >
                  Table
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink
                  className={options.type === 'graph' ? 'active' : ''}
                  onClick={() => this.handleChangeType(PanelType.Graph)}
                >
                  Graph
                </NavLink>
              </NavItem>
              {!this.state.loading && !this.state.error && this.state.stats && <QueryStatsView {...this.state.stats} />}
            </Nav>
            <TabContent activeTab={options.type}>
              <TabPane tabId="table">
                {options.type === 'table' && (
                  <>
                    <div className="table-controls">
                      <TimeInput
                        time={options.endTime}
                        useLocalTime={this.props.useLocalTime}
                        range={options.range}
                        placeholder="Evaluation time"
                        onChangeTime={this.handleChangeEndTime}
                      />
                    </div>
                    <DataTable data={this.state.data} />
                  </>
                )}
              </TabPane>
              <TabPane tabId="graph">
                {this.props.options.type === 'graph' && (
                  <>
                    <GraphControls
                      range={options.range}
                      endTime={options.endTime}
                      useLocalTime={this.props.useLocalTime}
                      resolution={options.resolution}
                      stacked={options.stacked}
                      onChangeRange={this.handleChangeRange}
                      onChangeEndTime={this.handleChangeEndTime}
                      onChangeResolution={this.handleChangeResolution}
                      onChangeStacking={this.handleChangeStacking}
                    />
                    <GraphTabContent
                      data={this.state.data}
                      stacked={options.stacked}
                      useLocalTime={this.props.useLocalTime}
                      lastQueryParams={this.state.lastQueryParams}
                    />
                  </>
                )}
              </TabPane>
            </TabContent>
          </Col>
        </Row>
        <Row>
          <Col>
            <Button className="float-right" color="link" onClick={this.props.removePanel} size="sm">
              Remove Panel
            </Button>
          </Col>
        </Row>
      </div>
    );
  }
}

export default Panel;
