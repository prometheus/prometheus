import React, { PureComponent } from 'react';

import metricToSeriesName from './MetricFomat';

interface LegendProps {
  series: any; // TODO: Type this.
}

class Legend extends PureComponent<LegendProps> {
  renderLegendItem(s: any) {
    const seriesName = metricToSeriesName(s.labels, false);
    return (
      <tr key={seriesName} className="legend-item">
        <td>
          <div className="legend-swatch" style={{backgroundColor: s.color}}></div>
        </td>
        <td>{seriesName}</td>
      </tr>
    );
  }

  render() {
    return (
      <table className="graph-legend">
        <tbody>
          {this.props.series.map((s: any) => {return this.renderLegendItem(s)})}
        </tbody>
      </table>
    );
  }
}

export default Legend;
