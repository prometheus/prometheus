import React, { PureComponent, SyntheticEvent } from 'react';
import SeriesName from '../SeriesName';
import { GraphSeries } from './Graph';

interface LegendProps {
  chartData: GraphSeries[];
  onLegendMouseOut: (ev: SyntheticEvent<HTMLDivElement>) => void;
  onSeriesToggle: (selected: number[], index: number) => void;
  onHover: (index: number) => (ev: SyntheticEvent<HTMLDivElement>) => void;
}

export class Legend extends PureComponent<LegendProps, { selectedIndexes: number[] }> {
  state = {
    selectedIndexes: [] as number[],
  };
  handleSeriesSelect = (index: number) => (ev: any) => {
    // TODO: add proper event type
    const { selectedIndexes } = this.state;

    let selected = [index];
    // Unselect.
    if (selectedIndexes.includes(index)) {
      selected = selectedIndexes.filter(idx => idx !== index);
    } else if (ev.ctrlKey) {
      selected =
        // Flip the logic - In case non is selected ctrl + click should deselect clicked series.
        selectedIndexes.length === 0
          ? this.props.chartData.reduce<number[]>((acc, _, i) => {
              return i === index ? acc : [...acc, i];
            }, [])
          : [...selectedIndexes, index]; // Select multiple.
    }

    this.setState({ selectedIndexes: selected });
    this.props.onSeriesToggle(selected, index);
  };
  render() {
    const { chartData, onLegendMouseOut, onHover } = this.props;
    const { selectedIndexes } = this.state;
    const canUseHover = chartData.length > 1 && selectedIndexes.length === 0;

    return (
      <div className="graph-legend" onMouseOut={canUseHover ? onLegendMouseOut : undefined}>
        {chartData.map(({ index, color, labels }) => (
          <div
            style={{ opacity: selectedIndexes.length === 0 || selectedIndexes.includes(index) ? 1 : 0.5 }}
            onClick={chartData.length > 1 ? this.handleSeriesSelect(index) : undefined}
            onMouseOver={canUseHover ? onHover(index) : undefined}
            key={index}
            className="legend-item"
          >
            <span className="legend-swatch" style={{ backgroundColor: color }}></span>
            <SeriesName labels={labels} format />
          </div>
        ))}
        {chartData.length > 1 && (
          <div className="pl-1 mt-1 text-muted" style={{ fontSize: 13 }}>
            Click: toggle series, CTRL + click: toggle multiple series
          </div>
        )}
      </div>
    );
  }
}
