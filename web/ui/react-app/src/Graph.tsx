import $ from 'jquery';
import React, { PureComponent } from 'react';
import ReactResizeDetector from 'react-resize-detector';

import { Alert } from 'reactstrap';

require('flot');
require('flot/source/jquery.flot.crosshair');
require('flot/source/jquery.flot.legend');
require('flot/source/jquery.flot.time');
require('flot/source/jquery.canvaswrapper');
require('jquery.flot.tooltip');

import Legend from './Legend';

var graphID = 0;
function getGraphID() {
  // TODO: This is ugly.
  return graphID++;
}

interface GraphProps {
  data: any; // TODO: Type this.
  stacked: boolean;
  queryParams: {
    startTime: number,
    endTime: number,
    resolution: number,
  } | null;
}

class Graph extends PureComponent<GraphProps> {
  private id: number = getGraphID();
  private chartRef = React.createRef<HTMLDivElement>();

  escapeHTML(str: string) {
    var entityMap: {[key: string]: string} = {
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;',
      '/': '&#x2F;'
    };

    return String(str).replace(/[&<>"'/]/g, function (s) {
      return entityMap[s];
    });
  }

  renderLabels(labels: {[key: string]: string}) {
    let labelStrings: string[] = [];
    for (let label in labels) {
      if (label !== '__name__') {
        labelStrings.push('<strong>' + label + '</strong>: ' + this.escapeHTML(labels[label]));
      }
    }
    return '<div class="labels">' + labelStrings.join('<br>') + '</div>';
  };

  formatValue = (y: number | null): string => {
    if (y === null) {
      return 'null';
    }
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
    throw Error("couldn't format a value, this is a bug");
  }

  getOptions(): any {
    return {
      grid: {
        hoverable: true,
        clickable: true,
        autoHighlight: true,
        mouseActiveRadius: 100,
      },
      legend: {
        show: false,
      },
      xaxis: {
        mode: 'time',
        showTicks: true,
        showMinorTicks: true,
        timeBase: 'milliseconds',
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
        content: (label: string, xval: number, yval: number, flotItem: any) => {
          const series = flotItem.series; // TODO: type this.
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

  // This was adapted from Flot's color generation code.
  getColors() {
    let colors = [];
    const colorPool = ["#edc240", "#afd8f8", "#cb4b4b", "#4da74d", "#9440ed"];
    const colorPoolSize = colorPool.length;
    let variation = 0;
    const neededColors = this.props.data.result.length;

    for (let i = 0; i < neededColors; i++) {
      const c = ($ as any).color.parse(colorPool[i % colorPoolSize] || "#666");

      // Each time we exhaust the colors in the pool we adjust
      // a scaling factor used to produce more variations on
      // those colors. The factor alternates negative/positive
      // to produce lighter/darker colors.

      // Reset the variation after every few cycles, or else
      // it will end up producing only white or black colors.

      if (i % colorPoolSize === 0 && i) {
        if (variation >= 0) {
          if (variation < 0.5) {
            variation = -variation - 0.2;
          } else variation = 0;
        } else variation = -variation;
      }

      colors[i] = c.scale('rgb', 1 + variation);
    }

    return colors;
  }

  getData() {
    const colors = this.getColors();

    return this.props.data.result.map((ts: any /* TODO: Type this*/, index: number) => {
      // Insert nulls for all missing steps.
      let data = [];
      let pos = 0;
      const params = this.props.queryParams!;

      for (let t = params.startTime; t <= params.endTime; t += params.resolution) {
        // Allow for floating point inaccuracy.
        if (ts.values.length > pos && ts.values[pos][0] < t + params.resolution / 100) {
          data.push([ts.values[pos][0] * 1000, this.parseValue(ts.values[pos][1])]);
          pos++;
        } else {
          // TODO: Flot has problems displaying intermittent "null" values when stacked,
          // resort to 0 now. In Grafana this works for some reason, figure out how they
          // do it.
          data.push([t * 1000, this.props.stacked ? 0 : null]);
        }
      }

      return {
        labels: ts.metric !== null ? ts.metric : {},
        data: data,
        color: colors[index],
        index: index,
      };
    })
  }

  parseValue(value: string) {
    var val = parseFloat(value);
    if (isNaN(val)) {
      // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). They
      // can't be graphed, so show them as gaps (null).

      // TODO: Flot has problems displaying intermittent "null" values when stacked,
      // resort to 0 now. In Grafana this works for some reason, figure out how they
      // do it.
      return this.props.stacked ? 0 : null;
    }
    return val;
  };

  componentDidMount() {
    this.plot();
  }

  componentDidUpdate() {
    this.plot();
  }

  componentWillUnmount() {
    this.destroyPlot();
  }

  plot() {
    if (this.chartRef.current === null) {
      return;
    }
    this.destroyPlot();
    $.plot($(this.chartRef.current!), this.getData(), this.getOptions());
  }

  destroyPlot() {
    const chart = $(this.chartRef.current!).data('plot');
    if (chart !== undefined) {
      chart.destroy();
    }
  }

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
        <ReactResizeDetector handleWidth onResize={() => this.plot()} />
        <div className="graph-chart" ref={this.chartRef} />
        <Legend series={this.getData()}/>
      </div>
    );
  }
}

export default Graph;
