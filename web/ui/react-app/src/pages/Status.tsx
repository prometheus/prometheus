import React, { FC, Fragment } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table, Alert } from 'reactstrap';
import useFetches from '../hooks/useFetches';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import PathPrefixProps from '../PathPrefixProps';

const ENDPOINTS = ['/api/v1/status/runtimeinfo', '/api/v1/status/buildinfo', '/api/v1/alertmanagers'];
const sectionTitles = ['Runtime Information', 'Build Information', 'Alertmanagers'];

interface StatusConfig {
  [k: string]: { title?: string; customizeValue?: (v: any) => any; customRow?: boolean; skip?: boolean };
}

type StatusPageState = Array<{ [k: string]: string }>;

export const statusConfig: StatusConfig = {
  startTime: { title: 'Start time', customizeValue: (v: string) => new Date(v).toUTCString() },
  CWD: { title: 'Working directory' },
  reloadConfigSuccess: {
    title: 'Configuration reload',
    customizeValue: (v: boolean) => (v ? 'Successful' : 'Unsuccessful'),
  },
  lastConfigTime: { title: 'Last successful configuration reload' },
  chunkCount: { title: 'Head chunks' },
  timeSeriesCount: { title: 'Head time series' },
  corruptionCount: { title: 'WAL corruptions' },
  goroutineCount: { title: 'Goroutines' },
  storageRetention: { title: 'Storage retention' },
  activeAlertmanagers: {
    customRow: true,
    customizeValue: (alertMgrs: { url: string }[]) => {
      return (
        <Fragment key="alert-managers">
          <tr>
            <th>Endpoint</th>
          </tr>
          {alertMgrs.map(({ url }) => {
            const { origin, pathname } = new URL(url);
            return (
              <tr key={url}>
                <td>
                  <a href={url}>{origin}</a>
                  {pathname}
                </td>
              </tr>
            );
          })}
        </Fragment>
      );
    },
  },
  droppedAlertmanagers: { skip: true },
};

const endpointsMemo: { [prefix: string]: string[] } = {};

const Status: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  if (!endpointsMemo[pathPrefix]) {
    // TODO: REMOVE THIS SUPER UGLY HACK IMMEDIATELY.
    // The problem is that there's an infinite reload loop if the endpoints array is
    // reconstructed on every render, as the dependency checking in useFetches()
    // then thinks that something has changed... the whole useFetches() should
    // probably removed and solved differently (within the component?) somehow.
    endpointsMemo[pathPrefix] = ENDPOINTS.map(ep => `${pathPrefix}${ep}`);
  }
  const { response: data, error, isLoading } = useFetches<StatusPageState[]>(endpointsMemo[pathPrefix]);
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
  return (
    <>
      {data
        ? data.map((statuses, i) => {
            return (
              <Fragment key={i}>
                <h2>{sectionTitles[i]}</h2>
                <Table className="h-auto" size="sm" bordered striped>
                  <tbody>
                    {Object.entries(statuses).map(([k, v]) => {
                      const { title = k, customizeValue = (val: any) => val, customRow, skip } = statusConfig[k] || {};
                      if (skip) {
                        return null;
                      }
                      if (customRow) {
                        return customizeValue(v);
                      }
                      return (
                        <tr key={k}>
                          <th className="capitalize-title" style={{ width: '35%' }}>
                            {title}
                          </th>
                          <td className="text-break">{customizeValue(v)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </Table>
              </Fragment>
            );
          })
        : null}
    </>
  );
};

export default Status;
