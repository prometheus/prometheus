import React, { Component } from 'react';

import { Alert, Button, Col, Nav, NavItem, NavLink, Row, TabContent, TabPane } from 'reactstrap';

import moment from 'moment-timezone';

import ExpressionInput from './ExpressionInput';
import CMExpressionInput from './CMExpressionInput';
import GraphControls from './GraphControls';
import { GraphTabContent } from './GraphTabContent';
import DataTable from './DataTable';
import TimeInput from './TimeInput';
import QueryStatsView, { QueryStats } from './QueryStatsView';
import { QueryParams, ExemplarData } from '../../types/types';
import { API_PATH } from '../../constants/constants';

interface PanelProps {
  options: PanelOptions;
  onOptionsChanged: (opts: PanelOptions) => void;
  useLocalTime: boolean;
  pastQueries: string[];
  metricNames: string[];
  removePanel: () => void;
  onExecuteQuery: (query: string) => void;
  pathPrefix: string;
  useExperimentalEditor: boolean;
  enableAutocomplete: boolean;
  enableHighlighting: boolean;
  enableLinter: boolean;
  id: string;
}

interface PanelState {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any; // TODO: Type data.
  exemplars: ExemplarData;
  lastQueryParams: QueryParams | null;
  loading: boolean;
  warnings: string[] | null;
  error: string | null;
  stats: QueryStats | null;
  exprInputValue: string;
}

export interface PanelOptions {
  expr: string;
  type: PanelType;
  range: number; // Range in milliseconds.
  endTime: number | null; // Timestamp in milliseconds.
  resolution: number | null; // Resolution in seconds.
  stacked: boolean;
  showExemplars: boolean;
}

export enum PanelType {
  Graph = 'graph',
  Table = 'table',
}

export const PanelDefaultOptions: PanelOptions = {
  type: PanelType.Table,
  expr: '',
  range: 60 * 60 * 1000,
  endTime: null,
  resolution: null,
  stacked: false,
  showExemplars: false,
};

class Panel extends Component<PanelProps, PanelState> {
  private abortInFlightFetch: (() => void) | null = null;

  constructor(props: PanelProps) {
    super(props);

    this.state = {
      data: null,
      exemplars: [],
      lastQueryParams: null,
      loading: false,
      warnings: null,
      error: null,
      stats: null,
      exprInputValue: props.options.expr,
    };
  }

  componentDidUpdate({ options: prevOpts }: PanelProps): void {
    const { endTime, range, resolution, showExemplars, type } = this.props.options;
    if (
      prevOpts.endTime !== endTime ||
      prevOpts.range !== range ||
      prevOpts.resolution !== resolution ||
      prevOpts.type !== type ||
      showExemplars !== prevOpts.showExemplars
    ) {
      this.executeQuery();
    }
  }

  componentDidMount(): void {
    this.executeQuery();
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  executeQuery = async (): Promise<any> => {
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
    const startTime = endTime - this.props.options.range / 1000;
    const resolution = this.props.options.resolution || Math.max(Math.floor(this.props.options.range / 250000), 1);
    const params: URLSearchParams = new URLSearchParams({
      query: expr,
    });

    let path: string;
    switch (this.props.options.type) {
      case 'graph':
        path = 'query_range';
        params.append('start', startTime.toString());
        params.append('end', endTime.toString());
        params.append('step', resolution.toString());
        break;
      case 'table':
        path = 'query';
        params.append('time', endTime.toString());
        break;
      default:
        throw new Error('Invalid panel type "' + this.props.options.type + '"');
    }

    let query;
    let exemplars;
    try {
      query = await fetch(`${this.props.pathPrefix}/${API_PATH}/${path}?${params}`, {
        cache: 'no-store',
        credentials: 'same-origin',
        signal: abortController.signal,
      }).then((resp) => resp.json());

      if (query.status !== 'success') {
        throw new Error(query.error || 'invalid response JSON');
      }

      if (this.props.options.type === 'graph' && this.props.options.showExemplars) {
        params.delete('step'); // Not needed for this request.
        exemplars = await fetch(`${this.props.pathPrefix}/${API_PATH}/query_exemplars?${params}`, {
          cache: 'no-store',
          credentials: 'same-origin',
          signal: abortController.signal,
        }).then((resp) => resp.json());

        if (exemplars.status !== 'success') {
          throw new Error(exemplars.error || 'invalid response JSON');
        }
      }

      let resultSeries = 0;
      if (query.data) {
        const { resultType, result } = query.data;
        if (resultType === 'scalar') {
          resultSeries = 1;
        } else if (result && result.length > 0) {
          resultSeries = result.length;
        }
      }

      this.setState({
        error: null,
        data: query.data,
        exemplars: exemplars?.data,
        warnings: query.warnings,
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
    } catch (err: unknown) {
      const error = err as Error;
      if (error.name === 'AbortError') {
        // Aborts are expected, don't show an error for them.
        return;
      }
      this.setState({
        error: 'Error executing query: ' + error.message,
        loading: false,
      });
    }
  };

  setOptions(opts: Partial<PanelOptions>): void {
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

  handleChangeEndTime = (endTime: number | null): void => {
    this.setOptions({ endTime: endTime });
  };

  handleChangeResolution = (resolution: number | null): void => {
    this.setOptions({ resolution: resolution });
  };

  handleChangeType = (type: PanelType): void => {
    if (this.props.options.type === type) {
      return;
    }

    this.setState({ data: null });
    this.setOptions({ type: type });
  };

  handleChangeStacking = (stacked: boolean): void => {
    this.setOptions({ stacked: stacked });
  };

  handleChangeShowExemplars = (show: boolean): void => {
    this.setOptions({ showExemplars: show });
  };

  handleTimeRangeSelection = (startTime: number, endTime: number): void => {
    this.setOptions({ range: endTime - startTime, endTime: endTime });
  };

  render(): JSX.Element {
    const { pastQueries, metricNames, options } = this.props;
    return (
      <div className="panel">
        <Row>
          <Col>
            {this.props.useExperimentalEditor ? (
              <CMExpressionInput
                value={this.state.exprInputValue}
                onExpressionChange={this.handleExpressionChange}
                executeQuery={this.executeQuery}
                loading={this.state.loading}
                enableAutocomplete={this.props.enableAutocomplete}
                enableHighlighting={this.props.enableHighlighting}
                enableLinter={this.props.enableLinter}
                queryHistory={pastQueries}
                metricNames={metricNames}
              />
            ) : (
              <ExpressionInput
                value={this.state.exprInputValue}
                onExpressionChange={this.handleExpressionChange}
                executeQuery={this.executeQuery}
                loading={this.state.loading}
                enableAutocomplete={this.props.enableAutocomplete}
                queryHistory={pastQueries}
                metricNames={metricNames}
              />
            )}
          </Col>
        </Row>
        <Row>
          <Col>{this.state.error && <Alert color="danger">{this.state.error}</Alert>}</Col>
        </Row>
        {this.state.warnings?.map((warning, index) => (
          <Row key={index}>
            <Col>{warning && <Alert color="warning">{warning}</Alert>}</Col>
          </Row>
        ))}
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
                      showExemplars={options.showExemplars}
                      onChangeRange={this.handleChangeRange}
                      onChangeEndTime={this.handleChangeEndTime}
                      onChangeResolution={this.handleChangeResolution}
                      onChangeStacking={this.handleChangeStacking}
                      onChangeShowExemplars={this.handleChangeShowExemplars}
                    />
                    <GraphTabContent
                      data={this.state.data}
                      exemplars={this.state.exemplars}
                      stacked={options.stacked}
                      useLocalTime={this.props.useLocalTime}
                      showExemplars={options.showExemplars}
                      lastQueryParams={this.state.lastQueryParams}
                      id={this.props.id}
                      handleTimeRangeSelection={this.handleTimeRangeSelection}
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
