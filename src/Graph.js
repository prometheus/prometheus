import React, { PureComponent } from 'react';

import { Alert } from 'reactstrap';

import ReactFlot from 'react-flot';
import '../node_modules/react-flot/flot/jquery.flot.time.min';
import '../node_modules/react-flot/flot/jquery.flot.crosshair.min';
import '../node_modules/react-flot/flot/jquery.flot.tooltip.min';
import '../node_modules/react-flot/flot/jquery.flot.stack.min';

import metricToSeriesName from './MetricFomat.js';

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
      return <Alert color="light">No data queried yet</Alert>;
    }

    if (this.props.data.resultType !== 'matrix') {
      return <Alert color="danger">Query result is of wrong type '{this.props.data.resultType}', should be 'matrix' (range vector).</Alert>;
    }

    if (this.props.data.result.length === 0) {
      return <Alert color="secondary">Empty query result</Alert>;
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

export default Graph;
