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
  removePanel: () => void;
  // TODO Put initial panel values here.
}

interface PanelState {
  expr: string;
  type: 'graph' | 'table';
  range: number;
  endTime: number | null;
  resolution: number | null;
  stacked: boolean;
  data: any; // TODO: Define data.
  lastQueryParams: { // TODO: Share these with Graph.tsx in a file.
    startTime: number,
    endTime: number,
    resolution: number,
  } | null;
  loading: boolean;
  error: string | null;
  stats: null; // TODO: Stats.
}

class Panel extends Component<PanelProps, PanelState> {
  private abortInFlightFetch: (() => void) | null = null;

  constructor(props: PanelProps) {
    super(props);

    this.state = {
      expr: 'rate(node_cpu_seconds_total[1m])',
      type: 'graph',
      range: 3600,
      endTime: null, // This is in milliseconds.
      resolution: null,
      stacked: false,
      data: null,
      lastQueryParams: null,
      loading: false,
      error: null,
      stats: null,
    };
  }

  componentDidUpdate(prevProps: PanelProps, prevState: PanelState) {
    if (prevState.type !== this.state.type ||
        prevState.range !== this.state.range ||
        prevState.endTime !== this.state.endTime ||
        prevState.resolution !== this.state.resolution) {

      if (prevState.type !== this.state.type) {
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
    if (this.state.expr === '') {
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
    const startTime = endTime - this.state.range;
    const resolution = this.state.resolution || Math.max(Math.floor(this.state.range / 250), 1);

    const url = new URL('http://demo.robustperception.io:9090/');//window.location.href);
    const params: {[key: string]: string} = {
      'query': this.state.expr,
    };

    switch (this.state.type) {
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
        throw new Error('Invalid panel type "' + this.state.type + '"');
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

  handleExpressionChange = (expr: string): void => {
    this.setState({expr: expr});
  }

  handleChangeRange = (range: number): void => {
    this.setState({range: range});
  }

  getEndTime = (): number | moment.Moment => {
    if (this.state.endTime === null) {
      return moment();
    }
    return this.state.endTime;
  }

  handleChangeEndTime = (endTime: number | null) => {
    this.setState({endTime: endTime});
  }

  handleChangeResolution = (resolution: number) => {
    // TODO: Where should we validate domain model constraints? In the parent's
    // change handler like here, or in the calling component?
    if (resolution > 0) {
      this.setState({resolution: resolution});
    }
  }

  // getEndDate = () => {
  //   var self = this;
  //   if (!self.endDate || !self.endDate.val()) {
  //     return moment();
  //   }
  //   return self.endDate.data('DateTimePicker').date();
  // };

  // getOrSetEndDate = () => {
  //   var self = this;
  //   var date = self.getEndDate();
  //   self.setEndDate(date);
  //   return date;
  // };

  // setEndDate = (date) => {
  //   var self = this;
  //   self.endDate.data('DateTimePicker').date(date);
  // };

  // increaseEnd = () => {
  //   var self = this;
  //   var newDate = moment(self.getOrSetEndDate());
  //   newDate.add(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
  //   self.setEndDate(newDate);
  //   self.submitQuery();
  // };

  // decreaseEnd = () => {
  //   var self = this;
  //   var newDate = moment(self.getOrSetEndDate());
  //   newDate.subtract(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
  //   self.setEndDate(newDate);
  //   self.submitQuery();
  // };

  handleChangeStacking = (stacked: boolean) => {
    this.setState({stacked: stacked});
  }

  render() {
    return (
      <>
        <Row>
          <Col>
            <ExpressionInput
              value={this.state.expr}
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
                  className={this.state.type === 'graph' ? 'active' : ''}
                  onClick={() => { this.setState({type: 'graph'}); }}
                >
                  Graph
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink
                  className={this.state.type === 'table' ? 'active' : ''}
                  onClick={() => { this.setState({type: 'table'}); }}
                >
                  Table
                </NavLink>
              </NavItem>
            </Nav>
            <TabContent activeTab={this.state.type}>
              <TabPane tabId="graph">
                {this.state.type === 'graph' &&
                  <>
                    <GraphControls
                      range={this.state.range}
                      endTime={this.state.endTime}
                      resolution={this.state.resolution}
                      stacked={this.state.stacked}

                      onChangeRange={this.handleChangeRange}
                      onChangeEndTime={this.handleChangeEndTime}
                      onChangeResolution={this.handleChangeResolution}
                      onChangeStacking={this.handleChangeStacking}
                    />
                    <Graph data={this.state.data} stacked={this.state.stacked} queryParams={this.state.lastQueryParams} />
                  </>
                }
              </TabPane>
              <TabPane tabId="table">
                {this.state.type === 'table' &&
                  <>
                    <div className="table-controls">
                      <TimeInput endTime={this.state.endTime} range={this.state.range} onChangeEndTime={this.handleChangeEndTime} />
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
            <Button className="float-right" color="link" onClick={this.props.removePanel}>Remove Panel</Button>
          </Col>
        </Row>
      </>
    );
  }
}

export default Panel;
