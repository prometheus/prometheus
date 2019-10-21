import React, { PureComponent } from 'react';
import './QueryStats.css';

interface QueryStatsProps {
    stats: {
        loadTime: number;
        resolution: number;
        totalTimeSeries: number;
    } | null;
    loading: boolean;
}

class QueryStats extends PureComponent<QueryStatsProps>{

    render() {
        const stats = this.props.stats;
        const loading = this.props.loading;

        return (
            <div className="query-stats">
                {(!loading && stats != null) &&
                    <span className="float-right">Load Time: {stats.loadTime}ms &ensp; Resolution: {stats.resolution} &ensp; Total Time Series: {stats.totalTimeSeries}</span>
                }
            </div>
        );
    }
}

export default QueryStats;