import React, { Component, PureComponent } from 'react';
import {
  Alert,
  Button,
  ButtonGroup,
  Col,
  Container,
  Form,
  InputGroup,
  InputGroupAddon,
  InputGroupText,
  Input,
  Nav,
  NavItem,
  NavLink,
  Row,
  TabContent,
  TabPane,
  Table,
} from 'reactstrap';

import ReactFlot from 'react-flot';
import '../node_modules/react-flot/flot/jquery.flot.time.min';
import '../node_modules/react-flot/flot/jquery.flot.crosshair.min';
import '../node_modules/react-flot/flot/jquery.flot.tooltip.min';
import '../node_modules/react-flot/flot/jquery.flot.stack.min';

import './App.css';

import Downshift from 'downshift';
import moment from 'moment-timezone';

import fuzzy from 'fuzzy';

import 'tempusdominus-core';
import 'tempusdominus-bootstrap-4';
import '../node_modules/tempusdominus-bootstrap-4/build/css/tempusdominus-bootstrap-4.min.css';

import { library, dom } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faSearch,
  faSpinner,
  faChevronLeft,
  faChevronRight,
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
  faClock,
  faCalendar,
  faArrowUp,
  faArrowDown,
  faCalendarCheck,
  faTimes,
} from '@fortawesome/free-solid-svg-icons';
library.add(
  faSearch,
  faSpinner,
  faChevronLeft,
  faChevronRight,
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
  faClock,
  faCalendar,
  faArrowUp,
  faArrowDown,
  faCalendarCheck,
  faTimes,
);
dom.watch() // Sadly needed to also replace <i> within the date picker, since it's not a React component.

class App extends Component {
  render() {
    return (
      <Container fluid={true}>
        <PanelList />
      </Container>
    );
  }
}

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

class Panel extends Component {
  constructor(props) {
    super(props);

    this.state = {
      expr: 'rate(node_cpu_seconds_total[1m])',
      type: 'graph', // TODO enum?
      range: 3600,
      endTime: null, // This is in milliseconds.
      resolution: null,
      stacked: false,
      data: null,
      error: null,
      stats: null,
    };

    this.handleExpressionChange = this.handleExpressionChange.bind(this);
  }

  componentDidUpdate(prevProps, prevState) {
    const needsRefresh = ['type', 'range', 'endTime', 'resolution'].some(v => {
      return prevState[v] !== this.state[v];
    })
    if (needsRefresh) {
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

  executeQuery = ()=> {
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

    let endTime = this.getEndTime() / 1000;
    let startTime = endTime - this.state.range;
    let resolution = this.state.resolution || Math.max(Math.floor(this.state.range / 250), 1);

    let url = new URL('http://demo.robustperception.io:9090/');//window.location.href);
    let params = {
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

    fetch(url, {cache: 'no-store', signal: abortController.signal})
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

  handleExpressionChange(expr) {
    //this.setState({expr: event.target.value});
    this.setState({expr: expr});
  }

  handleChangeRange = (range) => {
    this.setState({range: range});
  }

  getEndTime = () => {
    if (this.state.endTime === null) {
      return moment();
    }
    return this.state.endTime;
  }

  handleChangeEndTime = (endTime) => {
    this.setState({endTime: endTime});
  }

  handleChangeResolution = (resolution) => {
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

  handleChangeStacking = (stacked) => {
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
              metrics={this.props.metrics}
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
                  <DataTable data={this.state.data} />
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

class ExpressionInput extends Component {
  handleKeyPress = (event) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.props.executeQuery();
      event.preventDefault();
    }
  }

  stateReducer = (state, changes) => {
    return changes;
    // // TODO: Remove this whole function if I don't notice any odd behavior without it.
    // // I don't remember why I had to add this and currently things seem fine without it.
    // switch (changes.type) {
    //   case Downshift.stateChangeTypes.keyDownEnter:
    //   case Downshift.stateChangeTypes.clickItem:
    //   case Downshift.stateChangeTypes.changeInput:
    //     return {
    //       ...changes,
    //       selectedItem: changes.inputValue,
    //     };
    //   default:
    //     return changes;
    // }
  }

  renderAutosuggest = (downshift) => {
    if (this.prevNoMatchValue && downshift.inputValue.includes(this.prevNoMatchValue)) {
      // TODO: Is this still correct with fuzzy?
      return null;
    }

    let matches = fuzzy.filter(downshift.inputValue.replace(/ /g, ''), this.props.metrics, {
      pre: "<strong>",
      post: "</strong>",
    });

    if (matches.length === 0) {
      this.prevNoMatchValue = downshift.inputValue;
      return null;
    }

    if (!downshift.isOpen) {
      return null; // TODO CHECK NEED FOR THIS
    }

    return (
      <ul className="autosuggest-dropdown" {...downshift.getMenuProps()}>
        {
          matches
            .slice(0, 200) // Limit DOM rendering to 100 results, as DOM rendering is sloooow.
            .map((item, index) => (
              <li
                {...downshift.getItemProps({
                  key: item.original,
                  index,
                  item: item.original,
                  style: {
                    backgroundColor:
                      downshift.highlightedIndex === index ? 'lightgray' : 'white',
                    fontWeight: downshift.selectedItem === item ? 'bold' : 'normal',
                  },
                })}
              >
                {/* TODO: Find better way than setting inner HTML dangerously. We just want the <strong> to not be escaped.
                    This will be a problem when we save history and the user enters HTML into a query. q*/}
                <span dangerouslySetInnerHTML={{__html: item.string}}></span>
              </li>
            ))
        }
      </ul>
    );
  }

  componentDidMount() {
    const $exprInput = window.$(this.exprInputRef);
    $exprInput.on('input', () => {
      const el = $exprInput.get(0);
      const offset = el.offsetHeight - el.clientHeight;
      $exprInput.css('height', 'auto').css('height', el.scrollHeight + offset);
    });
  }

  render() {
    return (
        <Downshift
          inputValue={this.props.value}
          onInputValueChange={this.props.onChange}
          selectedItem={this.props.value}
        >
          {downshift => (
            <div>
              <InputGroup className="expression-input">
                <InputGroupAddon addonType="prepend">
                  <InputGroupText>
                  {this.props.loading ? <FontAwesomeIcon icon="spinner" spin/> : <FontAwesomeIcon icon="search"/>}
                  </InputGroupText>
                </InputGroupAddon>

                <Input
                  autoFocus
                  type="textarea"
                  rows={1}
                  onKeyPress={this.handleKeyPress}
                  placeholder="Expression (press Shift+Enter for newlines)"
                  innerRef={ref => this.exprInputRef = ref}
                  //onChange={selection => alert(`You selected ${selection}`)}
                  {...downshift.getInputProps({
                    onKeyDown: event => {
                      switch (event.key) {
                        case 'Home':
                        case 'End':
                          // We want to be able to jump to the beginning/end of the input field.
                          // By default, Downshift otherwise jumps to the first/last suggestion item instead.
                          event.nativeEvent.preventDownshiftDefault = true;
                          break;
                        case 'Enter':
                          downshift.closeMenu();
                          break;
                        default:
                      }
                    }
                  })}
                />
                <InputGroupAddon addonType="append">
                  <Button className="execute-btn" color="primary" onClick={this.props.executeQuery}>Execute</Button>
                </InputGroupAddon>
              </InputGroup>
              {this.renderAutosuggest(downshift)}
            </div>
          )}
        </Downshift>
    );
  }
}

class DataTable extends PureComponent {
  limitSeries(series) {
    const maxSeries = 10000;

    if (series.length > maxSeries) {
      return series.slice(0, maxSeries);
    }
    return series;
  }

  render() {
    const data = this.props.data;

    if (data === null) {
      return <Alert color="light">No data queried yet</Alert>;
    }

    if (data.result === null || data.result.length === 0) {
      return <Alert color="secondary">Empty query result</Alert>;
    }

    let rows = [];
    let limitedSeries = this.limitSeries(data.result);
    if (data) {
      switch(data.resultType) {
        case 'vector':
          rows = limitedSeries.map((s, index) => {
            return <tr key={index}><td>{metricToSeriesName(s.metric)}</td><td>{s.value[1]}</td></tr>
          });
          break;
        case 'matrix':
          rows = limitedSeries.map((s, index) => {
            const valueText = s.values.map((v) => {
              return [1] + ' @' + v[0];
            }).join('\n');
            return <tr style={{whiteSpace: 'pre'}} key={index}><td>{metricToSeriesName(s.metric)}</td><td>{valueText}</td></tr>
          });
          break;
        case 'scalar':
          rows.push(<tr><td>scalar</td><td>{data.result[1]}</td></tr>);
          break;
        case 'string':
          rows.push(<tr><td>scalar</td><td>{data.result[1]}</td></tr>);
          break;
        default:
          return <Alert color="danger">Unsupported result value type '{data.resultType}'</Alert>;
      }
    }

    return (
      <>
        {data.result.length !== limitedSeries.length &&
          <Alert color="danger">
            <strong>Warning:</strong> Fetched {data.result.length} metrics, only displaying first {limitedSeries.length}.
          </Alert>
        }
        <Table hover size="sm" className="data-table">
          <tbody>
            {rows}
          </tbody>
        </Table>
      </>
    );
  }
}
class GraphControls extends Component {
  constructor(props) {
    super(props);

    this.state = {
      startDate: Date.now(),
    };

    this.rangeRef = React.createRef();
    this.endTimeRef = React.createRef();
    this.resolutionRef = React.createRef();
  }

  rangeUnits = {
    'y': 60 * 60 * 24 * 365,
    'w': 60 * 60 * 24 * 7,
    'd': 60 * 60 * 24,
    'h': 60 * 60,
    'm': 60,
    's': 1
  };

  rangeSteps = [
    '1s', '10s', '1m', '5m', '15m', '30m', '1h', '2h', '6h', '12h', '1d', '2d',
    '1w', '2w', '4w', '8w', '1y', '2y'
  ];

  parseRange(rangeText) {
    var rangeRE = new RegExp('^([0-9]+)([ywdhms]+)$');
    var matches = rangeText.match(rangeRE);
    if (!matches || matches.length !== 3) {
      return null;
    }
    var value = parseInt(matches[1]);
    var unit = matches[2];
    return value * this.rangeUnits[unit];
  }

  formatRange(range) {
    for (let unit of Object.keys(this.rangeUnits)) {
      if (range % this.rangeUnits[unit] === 0) {
        return (range / this.rangeUnits[unit]) + unit;
      }
    }
    return range + 's';
  }

  onChangeRangeInput = (rangeText) => {
    const range = this.parseRange(rangeText);
    if (range === null) {
      this.changeRangeInput(this.formatRange(this.props.range));
    } else {
      this.props.onChangeRange(this.parseRange(rangeText));
    }
  }

  changeRangeInput = (rangeText) => {
    this.rangeRef.current.value = rangeText;
  }

  increaseRange = (event) => {
    for (let range of this.rangeSteps) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range < rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
  }

  decreaseRange = (event) => {
    for (let range of this.rangeSteps.slice().reverse()) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range > rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
  }

  getBaseEndTime = () => {
    return this.props.endTime || moment();
  }

  increaseEndTime = (event) => {
    const endTime = moment(this.getBaseEndTime() + this.props.range*1000/2);
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', endTime);
  }

  decreaseEndTime = (event) => {
    const endTime = moment(this.getBaseEndTime() - this.props.range*1000/2);
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', endTime);
  }

  clearEndTime = (event) => {
    this.props.onChangeEndTime(null);
    this.$endTime.datetimepicker('date', null);
  }

  componentDidMount() {
    this.$endTime = window.$(this.endTimeRef.current);

    this.$endTime.datetimepicker({
      icons: {
        today: 'fas fa-calendar-check',
      },
      buttons: {
        //showClear: true,
        showClose: true,
        showToday: true,
      },
      sideBySide: true,
      format: 'YYYY-MM-DD HH:mm:ss',
      locale: 'en',
      timeZone: 'UTC',
      defaultDate: this.props.endTime,
    });

    this.$endTime.on('change.datetimepicker', e => {
      console.log("CHANGE", e)
      if (e.date) {
        this.props.onChangeEndTime(e.date);
      } else {
        this.$endTime.datetimepicker('date', e.target.value);
      }
    });
  }

  componentWillUnmount() {
    this.$endTime.datetimepicker('destroy');
  }

  render() {
    return (
      <Form inline className="graph-controls" onSubmit={e => e.preventDefault()}>
        <InputGroup className="range-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease range" onClick={this.decreaseRange}><FontAwesomeIcon icon="minus" fixedWidth/></Button>
          </InputGroupAddon>

          {/* <Input value={this.state.rangeInput} onChange={(e) => this.changeRangeInput(e.target.value)}/> */}
          <Input
            defaultValue={this.formatRange(this.props.range)}
            innerRef={this.rangeRef}
            onBlur={() => this.onChangeRangeInput(this.rangeRef.current.value)}
          />

          <InputGroupAddon addonType="append">
            <Button title="Increase range" onClick={this.increaseRange}><FontAwesomeIcon icon="plus" fixedWidth/></Button>
          </InputGroupAddon>
        </InputGroup>

        <InputGroup className="endtime-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease end time" onClick={this.decreaseEndTime}><FontAwesomeIcon icon="chevron-left" fixedWidth/></Button>
          </InputGroupAddon>

          <Input
            placeholder="End time"
            // value={this.props.endTime ? this.props.endTime : ''}
            innerRef={this.endTimeRef}
            // onChange={this.props.onChangeEndTime}
            onFocus={() => this.$endTime.datetimepicker('show')}
            onBlur={() => this.$endTime.datetimepicker('hide')}
            onKeyDown={(e) => ['Escape', 'Enter'].includes(e.key) && this.$endTime.datetimepicker('hide')}
          />

          {/* CAUTION: While the datetimepicker also has an option to show a 'clear' button,
              that functionality is broken, so we create an external solution instead. */}
          {this.props.endTime &&
            <InputGroupAddon addonType="append">
              <Button className="clear-endtime-btn" title="Clear end time" onClick={this.clearEndTime}><FontAwesomeIcon icon="times" fixedWidth/></Button>
            </InputGroupAddon>
          }

          <InputGroupAddon addonType="append">
            <Button title="Increase end time" onClick={this.increaseEndTime}><FontAwesomeIcon icon="chevron-right" fixedWidth/></Button>
          </InputGroupAddon>
        </InputGroup>

        <Input
          placeholder="Res. (s)"
          className="resolution-input"
          defaultValue={this.props.resolution !== null ? this.props.resolution : ''}
          innerRef={this.resolutionRef}
          onBlur={() => this.props.onChangeResolution(parseInt(this.resolutionRef.current.value))}
          bsSize="sm"
        />

        <ButtonGroup className="stacked-input" size="sm">
          <Button title="Show unstacked line graph" onClick={() => this.props.onChangeStacking(false)} active={!this.props.stacked}><FontAwesomeIcon icon="chart-line" fixedWidth/></Button>
          <Button title="Show stacked graph" onClick={() => this.props.onChangeStacking(true)} active={this.props.stacked}><FontAwesomeIcon icon="chart-area" fixedWidth/></Button>
        </ButtonGroup>
      </Form>
    );
  }
}

var graphID = 0;
function getGraphID() {
  // TODO: This is ugly.
  return graphID++;
}

class Graph extends PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      legendRef: null,
    };
    this.id = getGraphID();
  }

  escapeHTML(string) {
    var entityMap = {
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;',
      '/': '&#x2F;'
    };

    return String(string).replace(/[&<>"'/]/g, function (s) {
      return entityMap[s];
    });
  }

  renderLabels(labels) {
    let labelStrings = [];
    for (let label in labels) {
      if (label !== '__name__') {
        labelStrings.push('<strong>' + label + '</strong>: ' + this.escapeHTML(labels[label]));
      }
    }
    return labels = '<div class="labels">' + labelStrings.join('<br>') + '</div>';
  };

  // axisUnits = [
  //   {unit: 'Y', factor: 1e24},
  //   {unit: 'Z', factor: 1e21},
  //   {unit: 'E', factor: 1e18},
  //   {unit: 'P', factor: 1e15},
  //   {unit: 'T', factor: 1e12},
  //   {unit: 'G', factor:  1e9},
  //   {unit: 'M', factor:  1e6},
  //   {unit: 'K', factor:  1e3},
  //   {unit: null,factor:    1},
  //   {unit: 'm', factor:  1e-3},
  //   {unit: 'µ', factor:  1e-6},
  //   {unit: 'n', factor:  1e-9},
  //   {unit: 'p', factor: 1e-12},
  //   {unit: 'f', factor: 1e-15},
  //   {unit: 'a', factor: 1e-18},
  //   {unit: 'z', factor: 1e-21},
  //   {unit: 'y', factor: 1e-24},
  // ]

  formatValue = (y) => {
    var abs_y = Math.abs(y);
    if (abs_y >= 1e24) {
      return (y / 1e24).toFixed(2) + "Y";
    } else if (abs_y >= 1e21) {
      return (y / 1e21).toFixed(2) + "Z";
    } else if (abs_y >= 1e18) {
      return (y / 1e18).toFixed(2) + "E";
    } else if (abs_y >= 1e15) {
      return (y / 1e15).toFixed(2) + "P";
    } else if (abs_y >= 1e12) {
      return (y / 1e12).toFixed(2) + "T";
    } else if (abs_y >= 1e9) {
      return (y / 1e9).toFixed(2) + "G";
    } else if (abs_y >= 1e6) {
      return (y / 1e6).toFixed(2) + "M";
    } else if (abs_y >= 1e3) {
      return (y / 1e3).toFixed(2) + "k";
    } else if (abs_y >= 1) {
      return y.toFixed(2)
    } else if (abs_y === 0) {
      return y.toFixed(2)
    } else if (abs_y <= 1e-24) {
      return (y / 1e-24).toFixed(2) + "y";
    } else if (abs_y <= 1e-21) {
      return (y / 1e-21).toFixed(2) + "z";
    } else if (abs_y <= 1e-18) {
      return (y / 1e-18).toFixed(2) + "a";
    } else if (abs_y <= 1e-15) {
      return (y / 1e-15).toFixed(2) + "f";
    } else if (abs_y <= 1e-12) {
      return (y / 1e-12).toFixed(2) + "p";
    } else if (abs_y <= 1e-9) {
      return (y / 1e-9).toFixed(2) + "n";
    } else if (abs_y <= 1e-6) {
      return (y / 1e-6).toFixed(2) + "µ";
    } else if (abs_y <=1e-3) {
      return (y / 1e-3).toFixed(2) + "m";
    } else if (abs_y <= 1) {
      return y.toFixed(2)
    }
  }

  getOptions() {
    return {
      // colors: [
      //   '#7EB26D', // 0: pale green
      //   '#EAB839', // 1: mustard
      //   '#6ED0E0', // 2: light blue
      //   '#EF843C', // 3: orange
      //   '#E24D42', // 4: red
      //   '#1F78C1', // 5: ocean
      //   '#BA43A9', // 6: purple
      //   '#705DA0', // 7: violet
      //   '#508642', // 8: dark green
      //   '#CCA300', // 9: dark sand
      //   '#447EBC',
      //   '#C15C17',
      //   '#890F02',
      //   '#0A437C',
      //   '#6D1F62',
      //   '#584477',
      //   '#B7DBAB',
      //   '#F4D598',
      //   '#70DBED',
      //   '#F9BA8F',
      //   '#F29191',
      //   '#82B5D8',
      //   '#E5A8E2',
      //   '#AEA2E0',
      //   '#629E51',
      //   '#E5AC0E',
      //   '#64B0C8',
      //   '#E0752D',
      //   '#BF1B00',
      //   '#0A50A1',
      //   '#962D82',
      //   '#614D93',
      //   '#9AC48A',
      //   '#F2C96D',
      //   '#65C5DB',
      //   '#F9934E',
      //   '#EA6460',
      //   '#5195CE',
      //   '#D683CE',
      //   '#806EB7',
      //   '#3F6833',
      //   '#967302',
      //   '#2F575E',
      //   '#99440A',
      //   '#58140C',
      //   '#052B51',
      //   '#511749',
      //   '#3F2B5B',
      //   '#E0F9D7',
      //   '#FCEACA',
      //   '#CFFAFF',
      //   '#F9E2D2',
      //   '#FCE2DE',
      //   '#BADFF4',
      //   '#F9D9F9',
      //   '#DEDAF7',
      // ],
      grid: {
        hoverable: true,
        clickable: true,
        autoHighlight: true,
        mouseActiveRadius: 100,
      },
      legend: {
        container: this.state.legendRef,
        labelFormatter: (s) => {return '&nbsp;&nbsp;' + s}
      },
      xaxis: {
        mode: 'time',
        showTicks: true,
        showMinorTicks: true,
        // min: (new Date()).getTime(),
        // max: (new Date(2000, 1, 1)).getTime(),
      },
      yaxis: {
        tickFormatter: this.formatValue,
      },
      crosshair: {
        mode: 'xy',
        color: '#bbb',
      },
      tooltip: {
        show: true,
        cssClass: 'graph-tooltip',
        content: (label, xval, yval, flotItem) => {
          const series = flotItem.series;
          var date = '<span class="date">' + new Date(xval).toUTCString() + '</span>';
          var swatch = '<span class="detail-swatch" style="background-color: ' + series.color + '"></span>';
          var content = swatch + (series.labels.__name__ || 'value') + ": <strong>" + yval + '</strong>';
          return date + '<br>' + content + '<br>' + this.renderLabels(series.labels);
        },
        defaultTheme: false,
        lines: true,
      },
      series: {
        stack: this.props.stacked,
        lines: {
          lineWidth: this.props.stacked ? 1 : 2,
          steps: false,
          fill: this.props.stacked,
        },
        shadowSize: 0,
      }
    };
  }

  getData() {
    return this.props.data.result.map(ts => {
      // Insert nulls for all missing steps.
      let data = [];
      let pos = 0;
      const params = this.props.queryParams;
      for (let t = params.startTime; t <= params.endTime; t += params.resolution) {
        // Allow for floating point inaccuracy.
        if (ts.values.length > pos && ts.values[pos][0] < t + params.resolution / 100) {
          data.push([ts.values[pos][0] * 1000, this.parseValue(ts.values[pos][1])]);
          pos++;
        } else {
          data.push([t * 1000, null]);
        }
      }

      return {
        label: ts.metric !== null ? metricToSeriesName(ts.metric, true) : 'scalar',
        labels: ts.metric !== null ? ts.metric : {},
        data: data,
      };
    })
  }

  parseValue(value) {
    var val = parseFloat(value);
    if (isNaN(val)) {
      // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). The
      // can't be graphed, so show them as gaps (null).
      return null;
    }
    return val;
  };

  render() {
    if (this.props.data === null) {
      return <TabPaneAlert color="light">No data queried yet</TabPaneAlert>;
    }

    if (this.props.data.resultType !== 'matrix') {
      return <TabPaneAlert color="danger">Query result is of wrong type '{this.props.data.resultType}', should be 'matrix' (range vector).</TabPaneAlert>;
    }

    if (this.props.data.result.length === 0) {
      return <TabPaneAlert color="secondary">Empty query result</TabPaneAlert>;
    }

    return (
      <div className="graph">
        {this.state.legendRef &&
          <ReactFlot
            id={this.id.toString()}
            data={this.getData()}
            options={this.getOptions()}
            height="500px"
            width="100%"
          />
        }

        {/* Really nasty hack below with setState to trigger a second render after the legend div starts to exist. */}
        <div className="graph-legend" ref={ref => {!this.state.legendRef && this.setState({legendRef: ref})}}></div>
      </div>
    );
  }
}

function metricToSeriesName(labels, formatHTML) {
  var tsName = (labels.__name__ || '') + "{";
  var labelStrings = [];
  for (var label in labels) {
    if (label !== '__name__') {
      labelStrings.push((formatHTML ? '<b>' : '') + label + (formatHTML ? '</b>' : '') + '="' + labels[label] + '"');
    }
  }
  tsName += labelStrings.join(', ') + '}';
  return tsName;
};

export default App;
