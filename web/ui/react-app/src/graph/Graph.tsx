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
  chartData: GraphSeries[];
  selectedSeriesIndexes: number[];
}

class Graph extends PureComponent<GraphProps, GraphState> {
  private chartRef = React.createRef<HTMLDivElement>();
  private $chart?: jquery.flot.plot;
  private rafID = 0;

  state = {
    chartData: normalizeData(this.props),
    selectedSeriesIndexes: [] as number[],
  };

  componentDidUpdate(prevProps: GraphProps) {
    const { data, stacked } = this.props;
    if (prevProps.data !== data) {
      this.setState({ selectedSeriesIndexes: [], chartData: normalizeData(this.props) }, this.plot);
    } else if (prevProps.stacked !== stacked) {
      const { selectedSeriesIndexes } = this.state;
      this.setState({ chartData: normalizeData(this.props) }, () => {
        if (selectedSeriesIndexes.length === 0) {
          this.plot();
        } else {
          this.plot(this.state.chartData.filter((_, i) => selectedSeriesIndexes.includes(i)));
        }
      });
    }
  }

  componentDidMount() {
    this.plot();
  }

  componentWillUnmount() {
    this.destroyPlot();
  }

  plot = (data: GraphSeries[] = this.state.chartData) => {
    if (!this.chartRef.current) {
      return;
    }
    this.destroyPlot();

    this.$chart = $.plot($(this.chartRef.current), data, getOptions(this.props.stacked));
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

  handleSeriesSelect = (index: number) => (ev: any) => {
    const { chartData, selectedSeriesIndexes } = this.state;

    let selected = [index];
    // Unselect.
    if (selectedSeriesIndexes.includes(index)) {
      selected = selectedSeriesIndexes.filter(idx => idx !== index);
    } else if (ev.ctrlKey) {
      selected = [...selectedSeriesIndexes, index]; // Select multiple.
    }

    this.setState({ selectedSeriesIndexes: selected });

    this.plot(
      selectedSeriesIndexes.length === 1 && selectedSeriesIndexes.includes(index)
        ? chartData.map(toHoverColor(index, this.props.stacked))
        : chartData.filter((_, i) => selected.includes(i)) // draw only selected
    );
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

  handleResize = () => {
    const data = this.$chart ? this.$chart.getData() : undefined;
    this.plot(data as GraphSeries[] | undefined);
  };

  render() {
    const { selectedSeriesIndexes, chartData } = this.state;
    const canUseHover = chartData.length > 1 && selectedSeriesIndexes.length === 0;

    return (
      <div className="graph">
        <ReactResizeDetector handleWidth onResize={this.handleResize} skipOnMount />
        <div className="graph-chart" ref={this.chartRef} />
        <div className="graph-legend" onMouseOut={canUseHover ? this.handleLegendMouseOut : undefined}>
          {chartData.map(({ index, color, labels }) => (
            <div
              style={{ opacity: selectedSeriesIndexes.length === 0 || selectedSeriesIndexes.includes(index) ? 1 : 0.5 }}
              onClick={chartData.length > 1 ? this.handleSeriesSelect(index) : undefined}
              onMouseOver={canUseHover ? this.handleSeriesHover(index) : undefined}
              key={index}
              className="legend-item"
            >
              <span className="legend-swatch" style={{ backgroundColor: color }}></span>
              <SeriesName labels={labels} format />
            </div>
          ))}
          {chartData.length > 1 && (
            <div className="pl-1 mt-1 text-muted" style={{ fontSize: 13 }}>
              Click: toogle serie, CTRL + click: toogle multiple series
            </div>
          )}
        </div>
      </div>
    );
  }
}

export default Graph;
