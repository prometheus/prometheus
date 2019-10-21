import React, { Component } from 'react';

import {
  Alert,
  Button,
  Col,
  Nav,
  NavItem,
  NavLink,
  Row,
  TabContent,
  TabPane,
} from 'reactstrap';

import moment from 'moment-timezone';

import ExpressionInput from './ExpressionInput';
import GraphControls from './GraphControls';
import Graph from './Graph';
import DataTable from './DataTable';
import TimeInput from './TimeInput';
import QueryStats from './QueryStats';

interface PanelProps {
  options: PanelOptions;
  onOptionsChanged: (opts: PanelOptions) => void;
  metricNames: string[];
  removePanel: () => void;
}

interface PanelState {
  data: any; // TODO: Type data.
  lastQueryParams: { // TODO: Share these with Graph.tsx in a file.
    startTime: number,
    endTime: number,
    resolution: number,
  } | null;
  loading: boolean;
  error: string | null;
  stats: {
    loadTime: number,
    resolution: number,
    totalTimeSeries: number,
  } | null,
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
}

class Panel extends Component<PanelProps, PanelState> {
  private abortInFlightFetch: (() => void) | null = null;

  constructor(props: PanelProps) {
    super(props);

    this.state = {
      data: null,
      lastQueryParams: null,
      loading: false,
      error: null,
      stats: null,
    };
  }

  componentDidUpdate(prevProps: PanelProps, prevState: PanelState) {
    const prevOpts = prevProps.options;
    const opts = this.props.options;
    if (prevOpts.type !== opts.type ||
        prevOpts.range !== opts.range ||
        prevOpts.endTime !== opts.endTime ||
        prevOpts.resolution !== opts.resolution) {

      if (prevOpts.type !== opts.type) {
        // If the other options change, we still want to show the old data until the new
        // query completes, but this is not a good idea when we actually change between
        // table and graph view, since not all queries work well in both.
        this.setState({data: null});
      }
      this.executeQuery(opts.expr);
    }
  }

  componentDidMount() {
    this.executeQuery(this.props.options.expr);
  }

  executeQuery = (expr: string): void => {
    const queryStart = Date.now();
    if (this.props.options.expr !== expr) {
      this.setOptions({expr: expr});
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
    this.setState({loading: true});

    const endTime = this.getEndTime().valueOf() / 1000; // TODO: shouldn't valueof only work when it's a moment?
    const startTime = endTime - this.props.options.range;
    const resolution = this.props.options.resolution || Math.max(Math.floor(this.props.options.range / 250), 1);
    const url = new URL(window.location.href);
    const params: {[key: string]: string} = {
      'query': expr,
    };

    switch (this.props.options.type) {
      case 'graph':
        url.pathname = '../../api/v1/query_range'
        Object.assign(params, {
          start: startTime,
          end: endTime,
          step: resolution,
        })
        // TODO path prefix here and elsewhere.
        break;
      case 'table':
        url.pathname = '../../api/v1/query'
        Object.assign(params, {
          time: endTime,
        })
        break;
      default:
        throw new Error('Invalid panel type "' + this.props.options.type + '"');
    }
    Object.keys(params).forEach(key => url.searchParams.append(key, params[key]))

    fetch(url.toString(), {cache: 'no-store', signal: abortController.signal})
    .then(resp => resp.json())
    .then(json => {
      if (json.status !== 'success') {
        throw new Error(json.error || 'invalid response JSON');
      }

      let totalTimeSeries = 0;
      if (json.data !== undefined) {
        if (json.data.resultType === "scalar") {
          totalTimeSeries = 1;
        } else if(json.data.result !== null) {
          totalTimeSeries = json.data.result.length;
        }
      }
      
      this.setState({
        error: null,
        data: json.data,
        lastQueryParams: {
          startTime: startTime,
          endTime: endTime,
          resolution: resolution,
        },
        stats: {
          loadTime: Date.now() - queryStart,
          resolution: resolution,
          totalTimeSeries: totalTimeSeries,
        },
        loading: false,
      });
      this.abortInFlightFetch = null;
    })
    .catch(error => {
      if (error.name === 'AbortError') {
        // Aborts are expected, don't show an error for them.
        return
      }
      this.setState({
        error: 'Error executing query: ' + error.message,
        loading: false,
      })
    });
  }

  setOptions(opts: object): void {
    const newOpts = {...this.props.options, ...opts};
    this.props.onOptionsChanged(newOpts);
  }

  handleExpressionChange = (expr: string): void => {
    this.setOptions({expr: expr});
  }

  handleChangeRange = (range: number): void => {
    this.setOptions({range: range});
  }

  getEndTime = (): number | moment.Moment => {
    if (this.props.options.endTime === null) {
      return moment();
    }
    return this.props.options.endTime;
  }

  handleChangeEndTime = (endTime: number | null) => {
    this.setOptions({endTime: endTime});
  }

  handleChangeResolution = (resolution: number | null) => {
    this.setOptions({resolution: resolution});
  }

  handleChangeStacking = (stacked: boolean) => {
    this.setOptions({stacked: stacked});
  }

  render() {
    return (
      <div className="panel">
        <Row>
          <Col>
            <ExpressionInput
              value={this.props.options.expr}
              executeQuery={this.executeQuery}
              loading={this.state.loading}
              metricNames={this.props.metricNames}
            />
          </Col>
        </Row>
        <Row>
          <Col>
            {this.state.error && <Alert color="danger">{this.state.error}</Alert>}
          </Col>
        </Row>
        <Row>
          <Col>
            <Nav tabs>
              <NavItem>
                <NavLink
                  className={this.props.options.type === 'table' ? 'active' : ''}
                  onClick={() => { this.setOptions({type: 'table'}); }}
                >
                  Table
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink
                  className={this.props.options.type === 'graph' ? 'active' : ''}
                  onClick={() => { this.setOptions({type: 'graph'}); }}
                >
                  Graph
                </NavLink>
              </NavItem>
              <QueryStats loading={this.state.loading} stats={this.state.stats} />
            </Nav>
            <TabContent activeTab={this.props.options.type}>
              <TabPane tabId="table">
                {this.props.options.type === 'table' &&
                  <>
                    <div className="table-controls">
                      <TimeInput
                        time={this.props.options.endTime}
                        range={this.props.options.range}
                        placeholder="Evaluation time"
                        onChangeTime={this.handleChangeEndTime}
                      />
                    </div>
                    <DataTable data={this.state.data} />
                  </>
                }
              </TabPane>
              <TabPane tabId="graph">
                {this.props.options.type === 'graph' &&
                  <>
                    <GraphControls
                      range={this.props.options.range}
                      endTime={this.props.options.endTime}
                      resolution={this.props.options.resolution}
                      stacked={this.props.options.stacked}

                      onChangeRange={this.handleChangeRange}
                      onChangeEndTime={this.handleChangeEndTime}
                      onChangeResolution={this.handleChangeResolution}
                      onChangeStacking={this.handleChangeStacking}
                    />
                    <Graph data={this.state.data} stacked={this.props.options.stacked} queryParams={this.state.lastQueryParams} />
                  </>
                }
              </TabPane>
            </TabContent>
          </Col>
        </Row>
        <Row>
          <Col>
            <Button className="float-right" color="link" onClick={this.props.removePanel} size="sm">Remove Panel</Button>
          </Col>          
        </Row>
      </div>
    );
  }
}

export default Panel;
