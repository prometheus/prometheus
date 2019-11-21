import $ from 'jquery';
import React, { PureComponent } from 'react';
import ReactResizeDetector from 'react-resize-detector';
import { Alert } from 'reactstrap';

import { escapeHTML } from './utils/html';
import SeriesName from './SeriesName';
import { Metric, QueryParams } from './types/types';
require('flot');
require('flot/source/jquery.flot.crosshair');
require('flot/source/jquery.flot.legend');
require('flot/source/jquery.flot.time');
require('flot/source/jquery.canvaswrapper');
require('jquery.flot.tooltip');

interface GraphProps {
  data: {
    resultType: string;
    result: Array<{ metric: Metric; values: [number, string][] }>;
  };
  stacked: boolean;
  queryParams: QueryParams | null;
}

export interface GraphSeries {
  labels: { [key: string]: string };
  color: string;
  normalizedColor: string;
  data: (number | null)[][]; // [x,y][]
  index: number;
}

interface GraphState {
  selectedSeriesIndex: number | null;
  hoveredSeriesIndex: number | null;
}

class Graph extends PureComponent<GraphProps, GraphState> {
  private chartRef = React.createRef<HTMLDivElement>();

  state = {
    selectedSeriesIndex: null,
    hoveredSeriesIndex: null,
  };

  formatValue = (y: number | null): string => {
    if (y === null) {
      return 'null';
    }
    const absY = Math.abs(y);
    if (absY >= 1e24) {
      return (y / 1e24).toFixed(2) + 'Y';
    } else if (absY >= 1e21) {
      return (y / 1e21).toFixed(2) + 'Z';
    } else if (absY >= 1e18) {
      return (y / 1e18).toFixed(2) + 'E';
    } else if (absY >= 1e15) {
      return (y / 1e15).toFixed(2) + 'P';
    } else if (absY >= 1e12) {
      return (y / 1e12).toFixed(2) + 'T';
    } else if (absY >= 1e9) {
      return (y / 1e9).toFixed(2) + 'G';
    } else if (absY >= 1e6) {
      return (y / 1e6).toFixed(2) + 'M';
    } else if (absY >= 1e3) {
      return (y / 1e3).toFixed(2) + 'k';
    } else if (absY >= 1) {
      return y.toFixed(2);
    } else if (absY === 0) {
      return y.toFixed(2);
    } else if (absY < 1e-23) {
      return (y / 1e-24).toFixed(2) + 'y';
    } else if (absY < 1e-20) {
      return (y / 1e-21).toFixed(2) + 'z';
    } else if (absY < 1e-17) {
      return (y / 1e-18).toFixed(2) + 'a';
    } else if (absY < 1e-14) {
      return (y / 1e-15).toFixed(2) + 'f';
    } else if (absY < 1e-11) {
      return (y / 1e-12).toFixed(2) + 'p';
    } else if (absY < 1e-8) {
      return (y / 1e-9).toFixed(2) + 'n';
    } else if (absY < 1e-5) {
      return (y / 1e-6).toFixed(2) + 'µ';
    } else if (absY < 1e-2) {
      return (y / 1e-3).toFixed(2) + 'm';
    } else if (absY <= 1) {
      return y.toFixed(2);
    }
    throw Error("couldn't format a value, this is a bug");
  };

  getOptions(): jquery.flot.plotOptions {
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
        content: (_, xval, yval, { series }): string => {
          const { labels, color } = series;
          return `
            <div class="date">${new Date(xval).toUTCString()}</div>
            <div>
              <span class="detail-swatch" style="background-color: ${color}" />
              <span>${labels.__name__ || 'value'}: <strong>${yval}</strong></span>
            <div>
            <div class="labels mt-1">
              ${Object.keys(labels)
                .map(k =>
                  k !== '__name__' ? `<div class="mb-1"><strong>${k}</strong>: ${escapeHTML(labels[k])}</div>` : ''
                )
                .join('')}
            </div>
          `;
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
      },
    };
  }

  // This was adapted from Flot's color generation code.
  getColors() {
    const colorPool = ['#edc240', '#afd8f8', '#cb4b4b', '#4da74d', '#9440ed'];
    const colorPoolSize = colorPool.length;
    let variation = 0;
    return this.props.data.result.map((_, i) => {
      // Each time we exhaust the colors in the pool we adjust
      // a scaling factor used to produce more variations on
      // those colors. The factor alternates negative/positive
      // to produce lighter/darker colors.

      // Reset the variation after every few cycles, or else
      // it will end up producing only white or black colors.

      if (i % colorPoolSize === 0 && i) {
        if (variation >= 0) {
          variation = variation < 0.5 ? -variation - 0.2 : 0;
        } else {
          variation = -variation;
        }
      }
      return $.color.parse(colorPool[i % colorPoolSize] || '#666').scale('rgb', 1 + variation);
    });
  }

  getData(): GraphSeries[] {
    const colors = this.getColors();
    const { hoveredSeriesIndex } = this.state;
    const { stacked, queryParams } = this.props;
    const { startTime, endTime, resolution } = queryParams!;
    return this.props.data.result.map((ts, index) => {
      // Insert nulls for all missing steps.
      const data = [];
      let pos = 0;

      for (let t = startTime; t <= endTime; t += resolution) {
        // Allow for floating point inaccuracy.
        const currentValue = ts.values[pos];
        if (ts.values.length > pos && currentValue[0] < t + resolution / 100) {
          data.push([currentValue[0] * 1000, this.parseValue(currentValue[1])]);
          pos++;
        } else {
          // TODO: Flot has problems displaying intermittent "null" values when stacked,
          // resort to 0 now. In Grafana this works for some reason, figure out how they
          // do it.
          data.push([t * 1000, stacked ? 0 : null]);
        }
      }
      const { r, g, b } = colors[index];

      return {
        labels: ts.metric !== null ? ts.metric : {},
        color: `rgba(${r}, ${g}, ${b}, ${hoveredSeriesIndex === null || hoveredSeriesIndex === index ? 1 : 0.3})`,
        normalizedColor: `rgb(${r}, ${g}, ${b}`,
        data,
        index,
      };
    });
  }

  parseValue(value: string) {
    const val = parseFloat(value);
    if (isNaN(val)) {
      // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). They
      // can't be graphed, so show them as gaps (null).

      // TODO: Flot has problems displaying intermittent "null" values when stacked,
      // resort to 0 now. In Grafana this works for some reason, figure out how they
      // do it.
      return this.props.stacked ? 0 : null;
    }
    return val;
  }

  componentDidUpdate(prevProps: GraphProps) {
    if (prevProps.data !== this.props.data) {
      this.setState({ selectedSeriesIndex: null });
    }
    this.plot();
  }

  componentWillUnmount() {
    this.destroyPlot();
  }

  plot = () => {
    if (!this.chartRef.current) {
      return;
    }
    const selectedData = this.getData()[this.state.selectedSeriesIndex!];
    this.destroyPlot();
    $.plot($(this.chartRef.current), selectedData ? [selectedData] : this.getData(), this.getOptions());
  };

  destroyPlot() {
    const chart = $(this.chartRef.current!).data('plot');
    if (chart !== undefined) {
      chart.destroy();
    }
  }

  handleSeriesSelect = (index: number) => () => {
    const { selectedSeriesIndex } = this.state;
    this.setState({ selectedSeriesIndex: selectedSeriesIndex !== index ? index : null });
  };

  handleSeriesHover = (index: number) => () => {
    this.setState({ hoveredSeriesIndex: index });
  };

  handleLegendMouseOut = () => this.setState({ hoveredSeriesIndex: null });

  render() {
    if (this.props.data === null) {
      return <Alert color="light">No data queried yet</Alert>;
    }

    if (this.props.data.resultType !== 'matrix') {
      return (
        <Alert color="danger">
          Query result is of wrong type '{this.props.data.resultType}', should be 'matrix' (range vector).
        </Alert>
      );
    }

    if (this.props.data.result.length === 0) {
      return <Alert color="secondary">Empty query result</Alert>;
    }

    const { selectedSeriesIndex } = this.state;
    const series = this.getData();
    const canUseHover = series.length > 1 && selectedSeriesIndex === null;

    return (
      <div className="graph">
        <ReactResizeDetector handleWidth onResize={this.plot} />
        <div className="graph-chart" ref={this.chartRef} />
        <div className="graph-legend" onMouseOut={canUseHover ? this.handleLegendMouseOut : undefined}>
          {series.map(({ index, normalizedColor, labels }) => (
            <div
              style={{ opacity: selectedSeriesIndex !== null && index !== selectedSeriesIndex ? 0.7 : 1 }}
              onClick={series.length > 1 ? this.handleSeriesSelect(index) : undefined}
              onMouseOver={canUseHover ? this.handleSeriesHover(index) : undefined}
              key={index}
              className="legend-item"
            >
              <span className="legend-swatch" style={{ backgroundColor: normalizedColor }}></span>
              <SeriesName labels={labels} format />
            </div>
          ))}
        </div>
      </div>
    );
  }
}

export default Graph;
