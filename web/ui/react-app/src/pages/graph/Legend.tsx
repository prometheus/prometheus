import React, { PureComponent, SyntheticEvent } from 'react';
import SeriesName from './SeriesName';
import { GraphSeries } from './Graph';

interface LegendProps {
  chartData: GraphSeries[];
  shouldReset: boolean;
  onLegendMouseOut: (ev: SyntheticEvent<HTMLDivElement>) => void;
  onSeriesToggle: (selected: number[], index: number) => void;
  onHover: (index: number) => (ev: SyntheticEvent<HTMLDivElement>) => void;
}

interface LegendState {
  selectedIndexes: number[];
}

export class Legend extends PureComponent<LegendProps, LegendState> {
  state = {
    selectedIndexes: [] as number[],
  };
  componentDidUpdate(prevProps: LegendProps): void {
    if (this.props.shouldReset && prevProps.shouldReset !== this.props.shouldReset) {
      this.setState({ selectedIndexes: [] });
    }
  }
  handleSeriesSelect =
    (index: number) =>
    (ev: React.MouseEvent<HTMLDivElement, MouseEvent>): void => {
      // TODO: add proper event type
      const { selectedIndexes } = this.state;

      let selected = [index];
      if (ev.ctrlKey || ev.metaKey) {
        const { chartData } = this.props;
        if (selectedIndexes.includes(index)) {
          selected = selectedIndexes.filter((idx) => idx !== index);
        } else {
          selected =
            // Flip the logic - In case none is selected ctrl + click should deselect clicked series.
            selectedIndexes.length === 0
              ? chartData.reduce<number[]>((acc, _, i) => (i === index ? acc : [...acc, i]), [])
              : [...selectedIndexes, index]; // Select multiple.
        }
      } else if (selectedIndexes.length === 1 && selectedIndexes.includes(index)) {
        selected = [];
      }

      this.setState({ selectedIndexes: selected });
      this.props.onSeriesToggle(selected, index);
    };

  render(): JSX.Element {
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
            Click: select series, {navigator.platform.includes('Mac') ? 'CMD' : 'CTRL'} + click: toggle multiple series
          </div>
        )}
      </div>
    );
  }
}
