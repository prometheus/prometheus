import React, { PureComponent } from 'react';

import SeriesName from './SeriesName';

interface LegendProps {
  series: any; // TODO: Type this.
}

class Legend extends PureComponent<LegendProps> {
  renderLegendItem(s: any) {
    return (
      <tr key={s.index} className="legend-item">
        <td>
          <div className="legend-swatch" style={{ backgroundColor: s.color }}></div>
        </td>
        <td>
          <SeriesName labels={s.labels} format={true} />
        </td>
      </tr>
    );
  }

  render() {
    return (
      <table className="graph-legend">
        <tbody>
          {this.props.series.map((s: any) => {
            return this.renderLegendItem(s);
          })}
        </tbody>
      </table>
    );
  }
}

export default Legend;
