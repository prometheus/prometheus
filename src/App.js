import React, { Component } from 'react';
import { Alert, Button, Col, Container, InputGroup, InputGroupAddon, Input, Nav, NavItem, NavLink, Row, TabContent, TabPane, Table } from 'reactstrap';
import ReactFlot from 'react-flot';
import '../node_modules/react-flot/flot/jquery.flot.time.min';
import './App.css';

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
    };
    this.key = 0;

    this.addPanel = this.addPanel.bind(this);
    this.removePanel = this.removePanel.bind(this);
  }

  componentDidMount() {
    this.addPanel();
  }

  getKey() {
    return (this.key++).toString();
  }

  addPanel() {
    const panels = this.state.panels.slice();
    const key = this.getKey();
    panels.push(<Panel key={key} removePanel={() => this.removePanel(key)}/>);
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
        {this.state.panels}
        <Button color="primary" onClick={this.addPanel}>Add Panel</Button>
      </>
    );
  }
}

class Panel extends Component {
  constructor(props) {
    super(props);

    this.state = {
      expr: '',
      type: 'table', // TODO enum?
      range: 3600,
      endTime: null,
      step: null,
      data: null,
      loading: false,
      error: null,
      stats: null,
    };

    this.execute = this.execute.bind(this);
    this.handleExpressionChange = this.handleExpressionChange.bind(this);
  }

  execute() {
    if (this.state.expr === "") {
      return;
    }

    this.setState({loading: true});

    let url = new URL('http://localhost:9090/');//window.location.href);
    let params = {
      'query': this.state.expr,
    };
    switch (this.state.type) {
      case 'graph':
        url.pathname = '/api/v1/query_range'
        Object.assign(params, {
          start: '1549134688',
          end: '1549135688',
          step: 10,
        })
        // TODO path prefix here and elsewhere.
        break;
      case 'table':
        url.pathname = '/api/v1/query'
        Object.assign(params, {
          time: '1549134688',
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
        console.log(resp);
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

  handleExpressionChange(event) {
    this.setState({expr: event.target.value});
  }

  render() {
    return (
      <>
        <Row>
          <Col>
            <ExpressionInput value={this.state.expr} onChange={this.handleExpressionChange} execute={this.execute}/>
          </Col>
        </Row>
        <Row>
          <Col>
            {this.state.loading && "Loading..."}
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
              // TODO: Only render this pane when it's selected.
              <TabPane tabId="graph">
                <GraphControls
                  range={this.state.range}
                  endTime={this.state.endTime}
                  step={this.state.step}
                />
                <Graph data={this.state.data} />
              </TabPane>
              <TabPane tabId="table">
                <DataTable data={this.state.data} />
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

  render() {
    return (
      <InputGroup className="expression-input">
        <Input
          autoFocus
          type="textarea"
          rows={this.numRows()}
          value={this.props.value}
          onChange={this.props.onChange}
          onKeyPress={this.handleKeyPress}
          placeholder="Expression (press Shift+Enter for newlines)" />
        <InputGroupAddon addonType="append">
          <Button color="primary" onClick={this.props.execute}>Execute</Button>
        </InputGroupAddon>
      </InputGroup>
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
  // TODO
}

var graphID = 0;
function getGraphID() {
  // TODO: This is ugly.
  return graphID++;
}

class Graph extends Component {
  componentDidMount() {
    this.chart = null;
  }

  getOptions() {
    return {
      grid: {
        hoverable: true,
        clickable: true,
      },
      legend: {
        container: this.legend,
      },
      xaxis: {
        mode: "time",
        //timeformat: "%Y/%m/%d",
      },
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
      <div>
        <ReactFlot id={getGraphID()} data={this.getData()} options={this.getOptions()} width="1900px" height="500px" />
        <div ref={ref => { this.legend = ref; }}></div>
      </div>
    );
  }
}

function metricToSeriesName(labels) {
  var tsName = (labels.__name__ || '') + "{";
  var labelStrings = [];
   for (var label in labels) {
     if (label !== "__name__") {
       labelStrings.push(label + "=\"" + labels[label] + "\"");
     }
   }
  tsName += labelStrings.join(",") + "}";
  return tsName;
};

export default App;
