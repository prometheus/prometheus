import React, { FC, ReactNode } from 'react';

import { Alert, Table } from 'reactstrap';

import SeriesName from './SeriesName';
import { Metric } from '../../types/types';

import moment from 'moment';

export interface DataTableProps {
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
  useLocalTime: boolean;
}

interface InstantSample {
  metric: Metric;
  value: SampleValue;
}

interface RangeSamples {
  metric: Metric;
  values: SampleValue[];
}

type SampleValue = [number, string];

const limitSeries = <S extends InstantSample | RangeSamples>(series: S[]): S[] => {
  const maxSeries = 10000;

  if (series.length > maxSeries) {
    return series.slice(0, maxSeries);
  }
  return series;
};

const DataTable: FC<DataTableProps> = ({ data, useLocalTime }) => {
  if (data === null) {
    return <Alert color="light">No data queried yet</Alert>;
  }

  if (data.result === null || data.result.length === 0) {
    return <Alert color="secondary">Empty query result</Alert>;
  }

  const maxFormattableSize = 1000;
  let rows: ReactNode[] = [];
  let limited = false;
  const doFormat = data.result.length <= maxFormattableSize;
  switch (data.resultType) {
    case 'vector':
      rows = (limitSeries(data.result) as InstantSample[]).map((s: InstantSample, index: number): ReactNode => {
        return (
          <tr key={index}>
            <td>
              <SeriesName labels={s.metric} format={doFormat} />
            </td>
            <td>{s.value[1]}</td>
          </tr>
        );
      });
      limited = rows.length !== data.result.length;
      break;
    case 'matrix':
      rows = (limitSeries(data.result) as RangeSamples[]).map((s, seriesIdx) => {
        const valuesAndTimes = s.values.map((v, valIdx) => {
          const printedDatetime = moment.unix(v[0]).toISOString(useLocalTime);
          return (
            <React.Fragment key={valIdx}>
              {v[1]} @{<span title={printedDatetime}>{v[0]}</span>}
              <br />
            </React.Fragment>
          );
        });
        return (
          <tr style={{ whiteSpace: 'pre' }} key={seriesIdx}>
            <td>
              <SeriesName labels={s.metric} format={doFormat} />
            </td>
            <td>{valuesAndTimes}</td>
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
          <td>string</td>
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
      {!doFormat && (
        <Alert color="secondary">
          <strong>Notice:</strong> Showing more than {maxFormattableSize} series, turning off label formatting for
          performance reasons.
        </Alert>
      )}
      <Table hover size="sm" className="data-table">
        <tbody>{rows}</tbody>
      </Table>
    </>
  );
};

export default DataTable;
