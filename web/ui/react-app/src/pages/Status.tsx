import React, { FC, Fragment } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table, Alert } from 'reactstrap';
import useFetches from '../hooks/useFetches';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

const ENDPOINTS = ['../api/v1/status/runtimeinfo', '../api/v1/status/buildinfo', '../api/v1/alertmanagers'];
const sectionTitles = ['Runtime Information', 'Build Information', 'Alertmanagers'];

interface StatusConfig {
  [k: string]: { title: string; normalizeValue?: (v: any) => any };
}

type StatusPageState = Array<{ [k: string]: string }>;

const normalizeAlertmanagerValue = (alertMgrs: { url: string }[]) => {
  return alertMgrs.map(({ url }) => (
    <a key={url} className="mr-5" href={url}>
      {url}
    </a>
  ));
};

export const statusConfig: StatusConfig = {
  StartTime: { title: 'Start time', normalizeValue: (v: string) => new Date(v).toUTCString() },
  CWD: { title: 'Working directory' },
  ReloadConfigSuccess: {
    title: 'Configuration reload',
    normalizeValue: (v: boolean) => (v ? 'Successful' : 'Unsuccessful'),
  },
  LastConfigTime: { title: 'Last successful configuration reload' },
  ChunkCount: { title: 'Head chunks' },
  TimeSeriesCount: { title: 'Head time series' },
  CorruptionCount: { title: 'WAL corruptions' },
  GoroutineCount: { title: 'Goroutines' },
  StorageRetention: { title: 'Storage retention' },
  activeAlertmanagers: {
    title: 'Active',
    normalizeValue: normalizeAlertmanagerValue,
  },
  droppedAlertmanagers: {
    title: 'Dropped',
    normalizeValue: normalizeAlertmanagerValue,
  },
};

const Status = () => {
  const { response: data, error, isLoading } = useFetches<StatusPageState[]>(ENDPOINTS);
  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching status: {error.message}
      </Alert>
    );
  } else if (isLoading) {
    return (
      <FontAwesomeIcon
        size="3x"
        icon={faSpinner}
        spin
        className="position-absolute"
        style={{ transform: 'translate(-50%, -50%)', top: '50%', left: '50%' }}
      />
    );
  }
  return data
    ? data.map((statuses, i) => {
        return (
          <Fragment key={i}>
            <h2>{sectionTitles[i]}</h2>
            <Table className="h-auto" size="sm" borderless striped>
              <tbody>
                {Object.entries(statuses).map(([k, v]) => {
                  const { title = k, normalizeValue = (val: any) => val } = statusConfig[k] || {};
                  return (
                    <tr key={k}>
                      <th style={{ width: '35%' }}>{title}</th>
                      <td className="text-break">{normalizeValue(v)}</td>
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
