import React, { FC } from 'react';

import SeriesName from './SeriesName';

interface LegendProps {
  series: any; // TODO: Type this.
}

const Legend: FC<LegendProps> = ({ series }) => {
  return (
    <table className="graph-legend">
      <tbody>
        {series.map((s: any) => (
          <tr key={s.index} className="legend-item">
            <td>
              <div className="legend-swatch" style={{ backgroundColor: s.color }}></div>
            </td>
            <td>
              <SeriesName labels={s.labels} format={true} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
};

export default Legend;
