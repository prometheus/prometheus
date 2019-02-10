import React, { Component } from 'react';
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
  Table
} from 'reactstrap';
import ReactFlot from 'react-flot';
import '../node_modules/react-flot/flot/jquery.flot.time.min';
import '../node_modules/react-flot/flot/jquery.flot.crosshair.min';
import '../node_modules/react-flot/flot/jquery.flot.tooltip.min';
import './App.css';
import Downshift from 'downshift';
import moment from 'moment'

import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons'

library.add(faSearch, faSpinner)

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
    };
    this.key = 0;

    this.addPanel = this.addPanel.bind(this);
    this.removePanel = this.removePanel.bind(this);
  }

  componentDidMount() {
    this.addPanel();

    fetch("http://demo.robustperception.io:9090/api/v1/label/__name__/values")
    .then(resp => {
      if (resp.ok) {
        return resp.json();
      } else {
        throw new Error('Unexpected response status when fetching metric names: ' + resp.statusText); // TODO extract error
      }
    })
    .then(json =>
      this.setState({ metrics: json.data })
    )
    .catch(error => {
      this.setState({error})
    });
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
        {this.state.panels.map(p =>
          <Panel key={p.key} removePanel={() => this.removePanel(p.key)} metrics={this.state.metrics}/>
        )}
        <Button color="primary" onClick={this.addPanel}>Add Panel</Button>
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
      endTime: null,
      resolution: null,
      stacked: false,
      data: null,
      loading: false,
      error: null,
      stats: null,
    };

    this.execute = this.execute.bind(this);
    this.handleExpressionChange = this.handleExpressionChange.bind(this);
  }

  componentDidUpdate(prevProps, prevState) {
    if (prevState.type !== this.state.type) {
      this.execute();
    }
  }

  componentDidMount() {
    this.execute();
  }

  execute() {
    // TODO: Abort existing queries.
    if (this.state.expr === "") {
      return;
    }

    this.setState({loading: true});

    let endTime = this.getEndTime() / 1000;
    let resolution = this.state.resolution || Math.max(Math.floor(this.state.range / 250), 1);

    let url = new URL('http://demo.robustperception.io:9090/');//window.location.href);
    let params = {
      'query': this.state.expr,
    };

    switch (this.state.type) {
      case 'graph':
        url.pathname = '/api/v1/query_range'
        Object.assign(params, {
          start: endTime - this.state.range,
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
        // TODO
    }
    Object.keys(params).forEach(key => url.searchParams.append(key, params[key]))

    fetch(url)
    .then(resp => {
      if (resp.ok) {
        return resp.json();
      } else {
        throw new Error('Unexpected response status: ' + resp.statusText);
      }
    })
    .then(json =>
      this.setState({
        error: null,
        data: json.data,
        loading: false,
      })
    )
    .catch(error => {
      this.setState({
        error,
        loading: false
      })
    });
  }

  handleExpressionChange(expr) {
    //this.setState({expr: event.target.value});
    this.setState({expr: expr});
  }

  timeFactors = {
    "y": 60 * 60 * 24 * 365,
    "w": 60 * 60 * 24 * 7,
    "d": 60 * 60 * 24,
    "h": 60 * 60,
    "m": 60,
    "s": 1
  };

  rangeSteps = [
    "1s", "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h", "1d", "2d",
    "1w", "2w", "4w", "8w", "1y", "2y"
  ];

  parseDuration(rangeText) {
    var rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
    var matches = rangeText.match(rangeRE);
    if (!matches) { return; }
    if (matches.length !== 3) {
      return 60;
    }
    var value = parseInt(matches[1]);
    var unit = matches[2];
    return value * this.timeFactors[unit];
  }

  increaseRange = (event) => {
    event.preventDefault();
    for (let range of this.rangeSteps) {
      let rangeSeconds = this.parseDuration(range);
      if (this.state.range < rangeSeconds) {
        this.setState({range: rangeSeconds}, this.execute)
        return;
      }
    }
  }

  decreaseRange = (event) => {
    event.preventDefault();
    for (let range of this.rangeSteps.slice().reverse()) {
      let rangeSeconds = this.parseDuration(range);
      if (this.state.range > rangeSeconds) {
        this.setState({range: rangeSeconds}, this.execute)
        return;
      }
    }
  }

  changeRange = (event) => {

  }

  increaseEndTime = () => {

  }

  decreaseEndTime = () => {

  }

  getEndTime = () => {
    if (this.state.endTime === null) {
      return moment();
    }
    return this.state.endTime();
  }

  changeEndTime = () => {

  }

  changeResolution = () => {

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

  changeStacking = (stacked) => {
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
              execute={this.execute}
              loading={this.state.loading}
              metrics={this.props.metrics}
            />
            {/*<Input type="select" name="selectMetric">
              {this.props.metrics.map(m => <option key={m}>{m}</option>)}
            </Input>*/}
          </Col>
        </Row>
        <Row>
          <Col>
            {/* {this.state.loading && "Loading..."} */}
          </Col>
        </Row>
        <Row>
          <Col>
            {this.state.error && <Alert color="danger">{this.state.error.toString()}</Alert>}
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

                      decreaseRange={this.decreaseRange}
                      increaseRange={this.increaseRange}
                      changeRange={this.changeRange}
                      decreaseEndTime={this.decreaseEndTime}
                      increaseEndTime={this.increaseEndTime}
                      changeEndTime={this.changeEndTime}
                      changeResolution={this.changeResolution}
                      changeStacking={this.changeStacking}
                    />
                    <Graph data={this.state.data} />
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
  constructor(props) {
    super(props);

    this.handleKeyPress = this.handleKeyPress.bind(this);
  }

  handleKeyPress(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.props.execute();
      event.preventDefault();
    }
  }

  numRows() {
    // TODO: Not ideal. This doesn't handle long lines.
    return this.props.value.split(/\r\n|\r|\n/).length;
  }

  stateReducer = (state, changes) => {
    switch (changes.type) {
      case Downshift.stateChangeTypes.keyDownEnter:
      case Downshift.stateChangeTypes.clickItem:
      case Downshift.stateChangeTypes.changeInput:
        return {
          ...changes,
          selectedItem: changes.inputValue,
        };
      default:
        return changes;
    }
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
                  rows={this.numRows()}
                  onKeyPress={this.handleKeyPress}
                  placeholder="Expression (press Shift+Enter for newlines)"
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
                  <Button className="execute-btn" color="primary" onClick={this.props.execute}>Execute</Button>
                </InputGroupAddon>
              </InputGroup>
              {downshift.isOpen &&
                <ul className="autosuggest-dropdown" {...downshift.getMenuProps()}>
                  {
                    this.props.metrics
                      .filter(item => !downshift.inputValue || item.includes(downshift.inputValue))
                      .slice(0, 100) // Limit DOM rendering to 100 results, as DOM rendering is sloooow.
                      .map((item, index) => (
                        <li
                          {...downshift.getItemProps({
                            key: item,
                            index,
                            item,
                            style: {
                              backgroundColor:
                                downshift.highlightedIndex === index ? 'lightgray' : 'white',
                              fontWeight: downshift.selectedItem === item ? 'bold' : 'normal',
                            },
                          })}
                        >
                          {item}
                        </li>
                      ))
                  }
                </ul>
              }
            </div>
          )}
        </Downshift>
    );
  }
}

function DataTable(props) {
  const data = props.data;
  var rows = <tr><td colSpan="2"><i>no data</i></td></tr>

  if (props.data) {
    switch(data.resultType) {
      case 'vector':
        if (data.result === null || data.result.length === 0) {
          break;
        }
        rows = props.data.result.map((s, index) => {
          return <tr key={index}><td>{metricToSeriesName(s.metric)}</td><td>{s.value[1]}</td></tr>
        });
        break;
      case 'matrix':
        if (data.result === null || data.result.length === 0) {
          break;
        }
        rows = props.data.result.map((s, index) => {
          const valueText = s.values.map((v) => {
            return [1] + ' @' + v[0];
          }).join('\n');
          return <tr key={index}><td>{metricToSeriesName(s.metric)}</td><td>{valueText}</td></tr>
        });
        break;
      default:
        // TODO
    }
  }

  return (
    <Table hover size="sm" className="data-table">
      <tbody>
        {rows}
      </tbody>
    </Table>
  );
}

class GraphControls extends Component {
  render() {
    return (
      <Form inline className="graph-controls">
        <InputGroup className="range-input">
          <InputGroupAddon addonType="prepend">
            <Button onClick={this.props.decreaseRange}>-</Button>
          </InputGroupAddon>

          <Input value={this.props.range} onChange={this.props.changeRange}/>

          <InputGroupAddon addonType="append">
            <Button onClick={this.props.increaseRange}>+</Button>
          </InputGroupAddon>
        </InputGroup>

        <InputGroup className="endtime-input">
          <InputGroupAddon addonType="prepend">
            <Button onClick={this.props.decreaseEndTime}>&lt;&lt;</Button>
          </InputGroupAddon>

          <Input value={this.props.endTime ? this.props.endTime : ''} onChange={this.props.changeEndTime} />

          <InputGroupAddon addonType="append">
            <Button onClick={this.props.increaseEndTime}>&gt;&gt;</Button>
          </InputGroupAddon>
        </InputGroup>

        <Input className="resolution-input" value={this.props.resolution ? this.props.resolution : ''} onChange={this.props.changeResolution} placeholder="Res. (s)"/>

        <ButtonGroup className="stacked-input">
          <Button onClick={() => this.props.changeStacking(false)} active={this.props.stacked}>stacked</Button>
          <Button onClick={() => this.props.changeStacking(true)} active={!this.props.stacked}>unstacked</Button>
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

class Graph extends Component {
  escapeHTML(string) {
    var entityMap = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': '&quot;',
      "'": '&#39;',
      "/": '&#x2F;'
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

  getOptions() {
    return {
      grid: {
        hoverable: true,
        clickable: true,
        autoHighlight: true,
        mouseActiveRadius: 100,
      },
      legend: {
        container: this.legend,
        labelFormatter: (s) => {return '&nbsp;&nbsp;' + s}
      },
      xaxis: {
        mode: 'time',
        showTicks: true,
        showMinorTicks: true,
        // min: (new Date()).getTime(),
        // max: (new Date(2000, 1, 1)).getTime(),
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
        lines: {
          lineWidth: 2,
          steps: false,
        },
        shadowSize: 0,
      }
    };
  }

  getData() {
    if (this.props.data.resultType !== 'matrix') {
      // TODO self.showError("Result is not of matrix type! Please enter a correct expression.");
      return [];
    }

    return this.props.data.result.map(ts => {
      return {
        label: metricToSeriesName(ts.metric),
        labels: ts.metric,
        data: ts.values.map(v => [v[0] * 1000, this.parseValue(v[1])]),
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
      return null;
    }
    return (
      <div className="graph">
        <ReactFlot
          id={getGraphID().toString()}
          data={this.getData()}
          options={this.getOptions()}
          height="500px"
          width="100%"
        />
        <div className="graph-legend" ref={ref => { this.legend = ref; }}></div>
      </div>
    );
  }
}

function metricToSeriesName(labels) {
  var tsName = (labels.__name__ || '') + "{";
  var labelStrings = [];
   for (var label in labels) {
     if (label !== "__name__") {
       labelStrings.push('<b>' + label + "</b>=\"" + labels[label] + "\"");
     }
   }
  tsName += labelStrings.join(", ") + "}";
  return tsName;
};

export default App;
