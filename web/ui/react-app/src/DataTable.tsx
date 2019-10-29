import React, { PureComponent, ReactNode } from 'react';

import { Alert, Table } from 'reactstrap';

import SeriesName from './SeriesName';

export interface QueryResult {
  data:
    | null
    | {
        resultType: 'vector';
        result: InstantSample[];
      }
    | {
        resultType: 'matrix';
        result: RangeSamples[];
      }
    | {
        resultType: 'scalar';
        result: SampleValue;
      }
    | {
        resultType: 'string';
        result: string;
      };
}

interface InstantSample {
  metric: Metric;
  value: SampleValue;
}

interface RangeSamples {
  metric: Metric;
  values: SampleValue[];
}

interface Metric {
  [key: string]: string;
}

type SampleValue = [number, string];

class DataTable extends PureComponent<QueryResult> {
  limitSeries(series: InstantSample[] | RangeSamples[]): InstantSample[] | RangeSamples[] {
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

    let rows: ReactNode[] = [];
    let limited = false;
    switch (data.resultType) {
      case 'vector':
        rows = (this.limitSeries(data.result) as InstantSample[]).map(
          (s: InstantSample, index: number): ReactNode => {
            return (
              <tr key={index}>
                <td>
                  <SeriesName labels={s.metric} format={false} />
                </td>
                <td>{s.value[1]}</td>
              </tr>
            );
          }
        );
        limited = rows.length !== data.result.length;
        break;
      case 'matrix':
        rows = (this.limitSeries(data.result) as RangeSamples[]).map((s, index) => {
          const valueText = s.values
            .map(v => {
              return [1] + ' @' + v[0];
            })
            .join('\n');
          return (
            <tr style={{ whiteSpace: 'pre' }} key={index}>
              <td>
                <SeriesName labels={s.metric} format={false} />
              </td>
              <td>{valueText}</td>
            </tr>
          );
        });
        limited = rows.length !== data.result.length;
        break;
      case 'scalar':
        rows.push(
          <tr key="0">
            <td>scalar</td>
            <td>{data.result[1]}</td>
          </tr>
        );
        break;
      case 'string':
        rows.push(
          <tr key="0">
            <td>scalar</td>
            <td>{data.result[1]}</td>
          </tr>
        );
        break;
      default:
        return <Alert color="danger">Unsupported result value type</Alert>;
    }

    return (
      <>
        {limited && (
          <Alert color="danger">
            <strong>Warning:</strong> Fetched {data.result.length} metrics, only displaying first {rows.length}.
          </Alert>
        )}
        <Table hover size="sm" className="data-table">
          <tbody>{rows}</tbody>
        </Table>
      </>
    );
  }
}

export default DataTable;
