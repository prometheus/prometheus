import React, { FC } from 'react';

export interface QueryStats {
  loadTime: number;
  resolution: number;
  resultSeries: number;
}

const QueryStatsView: FC<QueryStats> = (props) => {
  const { loadTime, resolution, resultSeries } = props;

  return (
    <div className="query-stats">
      <span className="float-right">
        Load time: {loadTime}ms &ensp; Resolution: {resolution}s &ensp; Result series: {resultSeries}
      </span>
    </div>
  );
};

export default QueryStatsView;
