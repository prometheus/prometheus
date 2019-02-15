import React, { PureComponent } from 'react';

import metricToSeriesName from './MetricFomat';

interface LegendProps {
  series: any; // TODO: Type this.
}

class Legend extends PureComponent<LegendProps> {
  renderLegendItem(s: any) {
    return (
      <div key={s.label} className="legend-item">
        <span className="legend-swatch">.</span>
        <span className="legend-label">{s.label}</span>
      </div>
    );
  }

  render() {
    return (
      <div className="graph-legend">
        {this.props.series.map((s: any) => {return this.renderLegendItem(s)})}
      </div>
    );
  }
}

export default Legend;
