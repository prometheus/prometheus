import React, { Component } from 'react';

class App extends Component {
  render() {
    return (
      <PanelList />
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
        <button onClick={this.addPanel}>+</button>
      </>
    );
  }
}

class Panel extends Component {
  constructor(props) {
    super(props);

    this.state = {
      expr: '',
      type: 'graph', // TODO enum?
      range: '1h',
      endTime: null,
      data: null,
      loading: false,
      error: null,
      stats: null,
    };

    this.execute = this.execute.bind(this);
    this.handleExpressionChange = this.handleExpressionChange.bind(this);
  }

  execute() {
    this.setState({loading: true});

    fetch('http://demo.robustperception.io:9090/api/v1/query?query=' + encodeURIComponent(this.state.expr))
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
      <div>
        <ExpressionInput value={this.state.expr} onChange={this.handleExpressionChange} execute={this.execute}/>
        {this.state.loading && "Loading..."}
        {this.state.error}
        <DataTable data={this.state.data} />
        <button onClick={this.props.removePanel}>-</button>
      </div>
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

  render() {
    return (
      <div>
        <textarea
          value={this.props.value}
          onChange={this.props.onChange}
          onKeyPress={this.handleKeyPress}
          placeholder="Expression (press Shift+Enter for newlines)" />
        <button onClick={this.props.execute}>Execute</button>
      </div>
    );
  }
}

function DataTable(props) {
  if (!props.data) {
    return <div>no data</div>;
  }
  const rows = props.data.result.map((s, index) => {
    return <tr key={index}><th>{metricToSeriesName(s.metric)}</th><td>{s.value[1]} @ {s.value[0]}</td></tr>
  });
  return (
    <table>
      <tbody>{rows}</tbody>
    </table>
  );
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
