import React, { FC, ReactNode } from 'react';

import { Alert, Button, ButtonGroup, Table } from 'reactstrap';

import SeriesName from './SeriesName';
import { Metric, Histogram } from '../../types/types';

import moment from 'moment';

import HistogramChart from './HistogramChart';

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
  const [scale, setScale] = React.useState<'linear' | 'exponential'>('exponential');

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
              {s.value && s.value[1]}
              {s.histogram && (
                <>
                  <HistogramChart histogram={s.histogram[1]} index={index} scale={scale} />
                  <div className="histogram-summary-wrapper">
                    <div className="histogram-summary">
                      <span>
                        <strong>Total count:</strong> {s.histogram[1].count}
                      </span>
                      <span>
                        <strong>Sum:</strong> {s.histogram[1].sum}
                      </span>
                    </div>
                    <div className="histogram-summary">
                      <span>x-axis scale:</span>
                      <ButtonGroup className="stacked-input" size="sm">
                        <Button
                          title="Show histogram on exponential scale"
                          onClick={() => setScale('exponential')}
                          active={scale === 'exponential'}
                        >
                          Exponential
                        </Button>
                        <Button
                          title="Show histogram on linear scale"
                          onClick={() => setScale('linear')}
                          active={scale === 'linear'}
                        >
                          Linear
                        </Button>
                      </ButtonGroup>
                    </div>
                  </div>
                  {histogramTable(s.histogram[1])}
                </>
              )}
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
                  {histogramTable(h[1])} @{<span title={printedDatetime}>{h[0]}</span>}
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

const leftDelim = (br: number): string => (br === 3 || br === 1 ? '[' : '(');
const rightDelim = (br: number): string => (br === 3 || br === 0 ? ']' : ')');

export const bucketRangeString = ([boundaryRule, leftBoundary, rightBoundary, _]: [
  number,
  string,
  string,
  string
]): string => {
  return `${leftDelim(boundaryRule)}${leftBoundary} -> ${rightBoundary}${rightDelim(boundaryRule)}`;
};

export const histogramTable = (h: Histogram): ReactNode => (
  <Table size="xs" responsive bordered>
    <thead>
      <tr>
        <th style={{ textAlign: 'center' }} colSpan={2}>
          Histogram Sample
        </th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <th>Range</th>
        <th>Count</th>
      </tr>
      {h.buckets?.map((b, i) => (
        <tr key={i}>
          <td>{bucketRangeString(b)}</td>
          <td>{b[3]}</td>
        </tr>
      ))}
    </tbody>
  </Table>
);
export default DataTable;
