import React, { FC, ReactNode } from 'react';

import { Alert, Table } from 'reactstrap';

import SeriesName from './SeriesName';
import { Metric, Histogram } from '../../types/types';

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
        result: SampleValue;
      };
  useLocalTime: boolean;
}

interface InstantSample {
  metric: Metric;
  value?: SampleValue;
  histogram?: SampleHistogram;
}

interface RangeSamples {
  metric: Metric;
  values?: SampleValue[];
  histograms?: SampleHistogram[];
}

type SampleValue = [number, string];
type SampleHistogram = [number, Histogram];

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
            <td>
              {s.value && s.value[1]} <HistogramString h={s.histogram && s.histogram[1]} />
            </td>
          </tr>
        );
      });
      limited = rows.length !== data.result.length;
      break;
    case 'matrix':
      rows = (limitSeries(data.result) as RangeSamples[]).map((s, seriesIdx) => {
        const valuesAndTimes = s.values
          ? s.values.map((v, valIdx) => {
              const printedDatetime = moment.unix(v[0]).toISOString(useLocalTime);
              return (
                <React.Fragment key={valIdx}>
                  {v[1]} @{<span title={printedDatetime}>{v[0]}</span>}
                  <br />
                </React.Fragment>
              );
            })
          : [];
        const histogramsAndTimes = s.histograms
          ? s.histograms.map((h, hisIdx) => {
              const printedDatetime = moment.unix(h[0]).toISOString(useLocalTime);
              return (
                <React.Fragment key={-hisIdx}>
                  <HistogramString h={h[1]} /> @{<span title={printedDatetime}>{h[0]}</span>}
                  <br />
                </React.Fragment>
              );
            })
          : [];
        return (
          <tr style={{ whiteSpace: 'pre' }} key={seriesIdx}>
            <td>
              <SeriesName labels={s.metric} format={doFormat} />
            </td>
            <td>
              {valuesAndTimes} {histogramsAndTimes}
            </td>
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

export interface HistogramStringProps {
  h?: Histogram;
}

export const HistogramString: FC<HistogramStringProps> = ({ h }) => {
  if (!h) {
    return <></>;
  }
  const buckets: string[] = [];

  if (h.buckets) {
    for (const bucket of h.buckets) {
      const left = bucket[0] === 3 || bucket[0] === 1 ? '[' : '(';
      const right = bucket[0] === 3 || bucket[0] === 0 ? ']' : ')';
      buckets.push(left + bucket[1] + ',' + bucket[2] + right + ':' + bucket[3] + ' ');
    }
  }

  return (
    <>
      {'{'} count:{h.count} sum:{h.sum} {buckets} {'}'}
    </>
  );
};

export default DataTable;
