import React, { FC, Fragment } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table, Alert } from 'reactstrap';
import useFetch from './useFetch';

const ENDPOINTS = ['../api/v1/runtimeinfo', '../api/v1/buildinfo', '../api/v1/alertmanagers'];
const sectionsTitles = ['Runtime Information', 'Build Information', 'Alertmanagers'];

interface StatusConfig {
  [k: string]: { title: string; normalizeValue?: (v: any) => any };
}

type StatusPageState = Array<{ [k: string]: string }>;

const normalizeAlertManagerValue = (alertMgrs: { url: string }[]) => {
  return alertMgrs.map(({ url }) => (
    <a key={url} className="mr-2" href={url}>
      {url}
    </a>
  ));
};

const statusConfig: StatusConfig = {
  Birth: { title: 'Uptime', normalizeValue: (v: string) => new Date(v).toUTCString() },
  CWD: { title: 'Working Directory' },
  ReloadConfigSuccess: {
    title: 'Configuration reload',
    normalizeValue: (v: boolean) => (v ? 'Successful' : 'Unsuccessful'),
  },
  LastConfigTime: { title: 'Last successful configuration reload' },
  ChunkCount: { title: 'Head chunks' },
  TimeSeriesCount: { title: 'Head time series' },
  CorruptionCount: { title: 'WAL corruptions' },
  GoroutineCount: { title: 'Goroutines' },
  StorageRetention: { title: 'Storage Retention' },
  activeAlertmanagers: {
    title: 'Active',
    normalizeValue: normalizeAlertManagerValue,
  },
  droppedAlertmanagers: {
    title: 'Dropped',
    normalizeValue: normalizeAlertManagerValue,
  },
};

const Status = () => {
  const { response: data, error, spinner } = useFetch<StatusPageState[]>(ENDPOINTS);
  console.log(data);
  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching status: {error.message}
      </Alert>
    );
  } else if (spinner) {
    return spinner;
  }
  return data
    ? data.map((statuses, i) => {
        return (
          <Fragment key={i}>
            <h2>{sectionsTitles[i]}</h2>
            <Table size="sm" key={i} borderless>
              <tbody>
                {Object.entries(statuses).map(([k, v], index: number) => {
                  const { title = k, normalizeValue = (val: any) => val } = statusConfig[k] || {};
                  return (
                    <tr key={k} className={index % 2 ? 'bg-white' : 'bg-light'}>
                      <td className="w-50">{title}</td>
                      <td>{normalizeValue(v)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
          </Fragment>
        );
      })
    : null;
};

export default Status as FC<RouteComponentProps>;
