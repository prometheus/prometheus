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

interface PanelProps {
  metricNames: string[];
  initialOptions?: PanelOptions | undefined;
  removePanel: () => void;
  // TODO Put initial panel values here.
}

interface PanelState {
  options: PanelOptions;
  data: any; // TODO: Type data.
  lastQueryParams: { // TODO: Share these with Graph.tsx in a file.
    startTime: number,
    endTime: number,
    resolution: number,
  } | null;
  loading: boolean;
  error: string | null;
  stats: null; // TODO: Stats.
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
  expr: 'rate(node_cpu_seconds_total[5m])',
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
      options: this.props.initialOptions ? this.props.initialOptions : {
        expr: '',
        type: PanelType.Table,
        range: 3600,
        endTime: null, // This is in milliseconds.
        resolution: null,
        stacked: false,
      },
      data: null,
      lastQueryParams: null,
      loading: false,
      error: null,
      stats: null,
    };
  }

  componentDidUpdate(prevProps: PanelProps, prevState: PanelState) {
    const prevOpts = prevState.options;
    const opts = this.state.options;
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
      this.executeQuery();
    }
  }

  componentDidMount() {
    this.executeQuery();
  }

  executeQuery = (): void => {
    if (this.state.options.expr === '') {
      return;
    }

    if (this.abortInFlightFetch) {
      this.abortInFlightFetch();
      this.abortInFlightFetch = null;
    }

    const abortController = new AbortController();
    this.abortInFlightFetch = () => abortController.abort();
    this.setState({loading: true});

    const endTime = this.getEndTime().valueOf() / 1000; // TODO: shouldn'T valueof only work when it's a moment?
    const startTime = endTime - this.state.options.range;
    const resolution = this.state.options.resolution || Math.max(Math.floor(this.state.options.range / 250), 1);

    const url = new URL('http://demo.robustperception.io:9090/');//window.location.href);
    const params: {[key: string]: string} = {
      'query': this.state.options.expr,
    };

    switch (this.state.options.type) {
      case 'graph':
        url.pathname = '/api/v1/query_range'
        Object.assign(params, {
          start: startTime,
          end: endTime,
          step: resolution,
        })
        // TODO path prefix here and elsewhere.
        break;
      case 'table':
        url.pathname = '/api/v1/query'
        Object.assign(params, {
          time: endTime,
        })
        break;
      default:
        throw new Error('Invalid panel type "' + this.state.options.type + '"');
    }
    Object.keys(params).forEach(key => url.searchParams.append(key, params[key]))

    fetch(url.toString(), {cache: 'no-store', signal: abortController.signal})
    .then(resp => resp.json())
    .then(json => {
      if (json.status !== 'success') {
        throw new Error(json.error || 'invalid response JSON');
      }

      this.setState({
        error: null,
        data: json.data,
        lastQueryParams: {
          startTime: startTime,
          endTime: endTime,
          resolution: resolution,
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
    const newOpts = Object.assign({}, this.state.options);
    this.setState({options: Object.assign(newOpts, opts)});
  }

  handleExpressionChange = (expr: string): void => {
    this.setOptions({expr: expr});
  }

  handleChangeRange = (range: number): void => {
    this.setOptions({range: range});
  }

  getEndTime = (): number | moment.Moment => {
    if (this.state.options.endTime === null) {
      return moment();
    }
    return this.state.options.endTime;
  }

  handleChangeEndTime = (endTime: number | null) => {
    this.setOptions({endTime: endTime});
  }

  handleChangeResolution = (resolution: number) => {
    // TODO: Where should we validate domain model constraints? In the parent's
    // change handler like here, or in the calling component?
    if (resolution > 0) {
      this.setOptions({resolution: resolution});
    }
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
              value={this.state.options.expr}
              onChange={this.handleExpressionChange}
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
                  className={this.state.options.type === 'graph' ? 'active' : ''}
                  onClick={() => { this.setOptions({type: 'graph'}); }}
                >
                  Graph
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink
                  className={this.state.options.type === 'table' ? 'active' : ''}
                  onClick={() => { this.setOptions({type: 'table'}); }}
                >
                  Table
                </NavLink>
              </NavItem>
            </Nav>
            <TabContent activeTab={this.state.options.type}>
              <TabPane tabId="graph">
                {this.state.options.type === 'graph' &&
                  <>
                    <GraphControls
                      range={this.state.options.range}
                      endTime={this.state.options.endTime}
                      resolution={this.state.options.resolution}
                      stacked={this.state.options.stacked}

                      onChangeRange={this.handleChangeRange}
                      onChangeEndTime={this.handleChangeEndTime}
                      onChangeResolution={this.handleChangeResolution}
                      onChangeStacking={this.handleChangeStacking}
                    />
                    <Graph data={this.state.data} stacked={this.state.options.stacked} queryParams={this.state.lastQueryParams} />
                  </>
                }
              </TabPane>
              <TabPane tabId="table">
                {this.state.options.type === 'table' &&
                  <>
                    <div className="table-controls">
                      <TimeInput endTime={this.state.options.endTime} range={this.state.options.range} onChangeEndTime={this.handleChangeEndTime} />
                    </div>
                    <DataTable data={this.state.data} />
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
