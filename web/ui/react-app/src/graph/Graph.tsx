import $ from 'jquery';
import React, { PureComponent } from 'react';
import ReactResizeDetector from 'react-resize-detector';

import SeriesName from '../SeriesName';
import { Metric, QueryParams } from '../types/types';
import { isPresent } from '../utils/func';
import { normalizeData, getOptions, toHoverColor } from './GraphHelpers';

require('flot');
require('flot/source/jquery.flot.crosshair');
require('flot/source/jquery.flot.legend');
require('flot/source/jquery.flot.time');
require('flot/source/jquery.canvaswrapper');
require('jquery.flot.tooltip');

export interface GraphProps {
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
  data: (number | null)[][]; // [x,y][]
  index: number;
}

interface GraphState {
  selectedSeriesIndex: number | null;
  chartData: GraphSeries[];
}

class Graph extends PureComponent<GraphProps, GraphState> {
  private chartRef = React.createRef<HTMLDivElement>();
  private $chart?: jquery.flot.plot;
  private rafID = 0;

  state = {
    selectedSeriesIndex: null,
    chartData: normalizeData(this.props),
  };

  componentDidUpdate(prevProps: GraphProps) {
    const { data, stacked } = this.props;
    if (prevProps.data !== data || prevProps.stacked !== stacked) {
      this.setState({ selectedSeriesIndex: null, chartData: normalizeData(this.props) }, this.plot);
    }
  }

  componentWillUnmount() {
    this.destroyPlot();
  }

  plot = () => {
    if (!this.chartRef.current) {
      return;
    }
    this.destroyPlot();

    this.$chart = $.plot($(this.chartRef.current), this.state.chartData, getOptions(this.props.stacked));
  };

  destroyPlot = () => {
    if (isPresent(this.$chart)) {
      this.$chart.destroy();
    }
  };

  plotSetAndDraw(data: GraphSeries[] = this.state.chartData) {
    if (isPresent(this.$chart)) {
      this.$chart.setData(data);
      this.$chart.draw();
    }
  }

  handleSeriesSelect = (index: number) => () => {
    const { selectedSeriesIndex, chartData } = this.state;
    this.plotSetAndDraw(
      selectedSeriesIndex === index
        ? chartData.map(toHoverColor(index, this.props.stacked))
        : chartData.slice(index, index + 1)
    );
    this.setState({ selectedSeriesIndex: selectedSeriesIndex === index ? null : index });
  };

  handleSeriesHover = (index: number) => () => {
    if (this.rafID) {
      cancelAnimationFrame(this.rafID);
    }
    this.rafID = requestAnimationFrame(() => {
      this.plotSetAndDraw(this.state.chartData.map(toHoverColor(index, this.props.stacked)));
    });
  };

  handleLegendMouseOut = () => {
    cancelAnimationFrame(this.rafID);
    this.plotSetAndDraw();
  };

  render() {
    const { selectedSeriesIndex, chartData } = this.state;
    const canUseHover = chartData.length > 1 && selectedSeriesIndex === null;

    return (
      <div className="graph">
        <ReactResizeDetector handleWidth onResize={this.plot} />
        <div className="graph-chart" ref={this.chartRef} />
        <div className="graph-legend" onMouseOut={canUseHover ? this.handleLegendMouseOut : undefined}>
          {chartData.map(({ index, color, labels }) => (
            <div
              style={{ opacity: selectedSeriesIndex === null || index === selectedSeriesIndex ? 1 : 0.5 }}
              onClick={chartData.length > 1 ? this.handleSeriesSelect(index) : undefined}
              onMouseOver={canUseHover ? this.handleSeriesHover(index) : undefined}
              key={index}
              className="legend-item"
            >
              <span className="legend-swatch" style={{ backgroundColor: color }}></span>
              <SeriesName labels={labels} format />
            </div>
          ))}
        </div>
      </div>
    );
  }
}

export default Graph;
