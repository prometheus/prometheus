import $ from 'jquery';
import React, { PureComponent } from 'react';
import ReactResizeDetector from 'react-resize-detector';

import { Legend } from './Legend';
import { Metric, Histogram, ExemplarData, QueryParams } from '../../types/types';
import { isPresent } from '../../utils';
import { normalizeData, getOptions, toHoverColor } from './GraphHelpers';
import { Button } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faTimes } from '@fortawesome/free-solid-svg-icons';

require('../../vendor/flot/jquery.flot');
require('../../vendor/flot/jquery.flot.stack');
require('../../vendor/flot/jquery.flot.time');
require('../../vendor/flot/jquery.flot.crosshair');
require('../../vendor/flot/jquery.flot.selection');
require('jquery.flot.tooltip');

export interface GraphProps {
  data: {
    resultType: string;
    result: Array<{ metric: Metric; values?: [number, string][]; histograms?: [number, Histogram][] }>;
  };
  exemplars: ExemplarData;
  stacked: boolean;
  useLocalTime: boolean;
  showExemplars: boolean;
  handleTimeRangeSelection: (startTime: number, endTime: number) => void;
  queryParams: QueryParams | null;
  id: string;
}

export interface GraphSeries {
  labels: { [key: string]: string };
  color: string;
  data: (number | null)[][]; // [x,y][]
  index: number;
}

export interface GraphExemplar {
  seriesLabels: { [key: string]: string };
  labels: { [key: string]: string };
  data: number[][];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  points: any; // This is used to specify the symbol.
  color: string;
}

export interface GraphData {
  series: GraphSeries[];
  exemplars: GraphExemplar[];
}

interface GraphState {
  chartData: GraphData;
  selectedExemplarLabels: { exemplar: { [key: string]: string }; series: { [key: string]: string } };
}

class Graph extends PureComponent<GraphProps, GraphState> {
  private chartRef = React.createRef<HTMLDivElement>();
  private $chart?: jquery.flot.plot;
  private rafID = 0;
  private selectedSeriesIndexes: number[] = [];

  state = {
    chartData: normalizeData(this.props),
    selectedExemplarLabels: { exemplar: {}, series: {} },
  };

  componentDidUpdate(prevProps: GraphProps): void {
    const { data, stacked, useLocalTime, showExemplars } = this.props;
    if (prevProps.data !== data) {
      this.selectedSeriesIndexes = [];
      this.setState({ chartData: normalizeData(this.props) }, this.plot);
    } else if (prevProps.stacked !== stacked) {
      this.setState({ chartData: normalizeData(this.props) }, () => {
        if (this.selectedSeriesIndexes.length === 0) {
          this.plot();
        } else {
          this.plot([
            ...this.state.chartData.series.filter((_, i) => this.selectedSeriesIndexes.includes(i)),
            ...this.state.chartData.exemplars,
          ]);
        }
      });
    }

    if (prevProps.useLocalTime !== useLocalTime) {
      this.plot();
    }

    if (prevProps.showExemplars !== showExemplars && !showExemplars) {
      this.setState(
        {
          chartData: { series: this.state.chartData.series, exemplars: [] },
          selectedExemplarLabels: { exemplar: {}, series: {} },
        },
        () => {
          this.plot();
        }
      );
    }
  }

  componentDidMount(): void {
    this.plot();

    $(`.graph-${this.props.id}`).bind('plotclick', (event, pos, item) => {
      // If an item has the series label property that means it's an exemplar.
      if (item && 'seriesLabels' in item.series) {
        this.setState({
          selectedExemplarLabels: { exemplar: item.series.labels, series: item.series.seriesLabels },
          chartData: this.state.chartData,
        });
      } else {
        this.setState({
          chartData: this.state.chartData,
          selectedExemplarLabels: { exemplar: {}, series: {} },
        });
      }
    });

    $(`.graph-${this.props.id}`).bind('plotselected', (_, ranges) => {
      if (isPresent(this.$chart)) {
        // eslint-disable-next-line
        // @ts-ignore Typescript doesn't think this method exists although it actually does.
        this.$chart.clearSelection();
        this.props.handleTimeRangeSelection(ranges.xaxis.from, ranges.xaxis.to);
      }
    });
  }

  componentWillUnmount(): void {
    this.destroyPlot();
  }

  plot = (
    data: (GraphSeries | GraphExemplar)[] = [...this.state.chartData.series, ...this.state.chartData.exemplars]
  ): void => {
    if (!this.chartRef.current) {
      return;
    }
    this.destroyPlot();

    this.$chart = $.plot($(this.chartRef.current), data, getOptions(this.props.stacked, this.props.useLocalTime));
  };

  destroyPlot = (): void => {
    if (isPresent(this.$chart)) {
      this.$chart.destroy();
    }
  };

  plotSetAndDraw(
    data: (GraphSeries | GraphExemplar)[] = [...this.state.chartData.series, ...this.state.chartData.exemplars]
  ): void {
    if (isPresent(this.$chart)) {
      this.$chart.setData(data);
      this.$chart.draw();
    }
  }

  handleSeriesSelect = (selected: number[], selectedIndex: number): void => {
    const { chartData } = this.state;
    this.plot(
      this.selectedSeriesIndexes.length === 1 && this.selectedSeriesIndexes.includes(selectedIndex)
        ? [...chartData.series.map(toHoverColor(selectedIndex, this.props.stacked)), ...chartData.exemplars]
        : [
            ...chartData.series.filter((_, i) => selected.includes(i)),
            ...chartData.exemplars.filter((exemplar) => {
              series: for (const i in selected) {
                for (const name in chartData.series[selected[i]].labels) {
                  if (exemplar.seriesLabels[name] !== chartData.series[selected[i]].labels[name]) {
                    continue series;
                  }
                }
                return true;
              }
              return false;
            }),
          ] // draw only selected
    );
    this.selectedSeriesIndexes = selected;
  };

  handleSeriesHover = (index: number) => (): void => {
    if (this.rafID) {
      cancelAnimationFrame(this.rafID);
    }
    this.rafID = requestAnimationFrame(() => {
      this.plotSetAndDraw([
        ...this.state.chartData.series.map(toHoverColor(index, this.props.stacked)),
        ...this.state.chartData.exemplars,
      ]);
    });
  };

  handleLegendMouseOut = (): void => {
    cancelAnimationFrame(this.rafID);
    this.plotSetAndDraw();
  };

  handleResize = (): void => {
    if (isPresent(this.$chart)) {
      this.plot(this.$chart.getData() as (GraphSeries | GraphExemplar)[]);
    }
  };

  render(): JSX.Element {
    const { chartData, selectedExemplarLabels } = this.state;
    const selectedLabels = selectedExemplarLabels as {
      exemplar: { [key: string]: string };
      series: { [key: string]: string };
    };
    return (
      <div className={`graph-${this.props.id}`}>
        <ReactResizeDetector handleWidth onResize={this.handleResize} skipOnMount />
        <div className="graph-chart" ref={this.chartRef} />
        {Object.keys(selectedLabels.exemplar).length > 0 ? (
          <div className="graph-selected-exemplar">
            <div className="font-weight-bold">Selected exemplar labels:</div>
            <div className="labels mt-1 ml-3">
              {Object.keys(selectedLabels.exemplar).map((k, i) => (
                <div key={i}>
                  <strong>{k}</strong>: {selectedLabels.exemplar[k]}
                </div>
              ))}
            </div>
            <div className="font-weight-bold mt-3">Associated series labels:</div>
            <div className="labels mt-1 ml-3">
              {Object.keys(selectedLabels.series).map((k, i) => (
                <div key={i}>
                  <strong>{k}</strong>: {selectedLabels.series[k]}
                </div>
              ))}
            </div>
            <Button
              size="small"
              color="light"
              style={{ position: 'absolute', top: 5, right: 5 }}
              title="Hide selected exemplar details"
              onClick={() =>
                this.setState({
                  chartData: this.state.chartData,
                  selectedExemplarLabels: { exemplar: {}, series: {} },
                })
              }
            >
              <FontAwesomeIcon icon={faTimes} />
            </Button>
          </div>
        ) : null}
        <Legend
          shouldReset={this.selectedSeriesIndexes.length === 0}
          chartData={chartData.series}
          onHover={this.handleSeriesHover}
          onLegendMouseOut={this.handleLegendMouseOut}
          onSeriesToggle={this.handleSeriesSelect}
        />
        {/* This is to make sure the graph box expands when the selected exemplar info pops up. */}
        <br style={{ clear: 'both' }} />
      </div>
    );
  }
}

export default Graph;
