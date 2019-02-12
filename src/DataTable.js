import React, { PureComponent } from 'react';

import { Alert, Table } from 'reactstrap';

import metricToSeriesName from './MetricFomat';

class DataTable extends PureComponent {
  limitSeries(series) {
    const maxSeries = 10000;

    if (series.length > maxSeries) {
      return series.slice(0, maxSeries);
    }
    return series;
  }

  render() {
    const data = this.props.data;

    if (data === null) {
      return <Alert color="light">No data queried yet</Alert>;
    }

    if (data.result === null || data.result.length === 0) {
      return <Alert color="secondary">Empty query result</Alert>;
    }

    let rows = [];
    let limitedSeries = this.limitSeries(data.result);
    if (data) {
      switch(data.resultType) {
        case 'vector':
          rows = limitedSeries.map((s, index) => {
            return <tr key={index}><td>{metricToSeriesName(s.metric)}</td><td>{s.value[1]}</td></tr>
          });
          break;
        case 'matrix':
          rows = limitedSeries.map((s, index) => {
            const valueText = s.values.map((v) => {
              return [1] + ' @' + v[0];
            }).join('\n');
            return <tr style={{whiteSpace: 'pre'}} key={index}><td>{metricToSeriesName(s.metric)}</td><td>{valueText}</td></tr>
          });
          break;
        case 'scalar':
          rows.push(<tr><td>scalar</td><td>{data.result[1]}</td></tr>);
          break;
        case 'string':
          rows.push(<tr><td>scalar</td><td>{data.result[1]}</td></tr>);
          break;
        default:
          return <Alert color="danger">Unsupported result value type '{data.resultType}'</Alert>;
      }
    }

    return (
      <>
        {data.result.length !== limitedSeries.length &&
          <Alert color="danger">
            <strong>Warning:</strong> Fetched {data.result.length} metrics, only displaying first {limitedSeries.length}.
          </Alert>
        }
        <Table hover size="sm" className="data-table">
          <tbody>
            {rows}
          </tbody>
        </Table>
      </>
    );
  }
}

export default DataTable;
