import React, { Fragment, FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { Fetch } from '../api/Fetch';

import PathPrefixProps from '../PathPrefixProps';
import { StatusIndicator } from '../StatusIndicator';

const endpoints = ['/api/v1/status/runtimeinfo', '/api/v1/status/buildinfo', '/api/v1/alertmanagers'];
const sectionTitles = ['Runtime Information', 'Build Information', 'Alertmanagers'];

interface StatusConfig {
  [k: string]: { title?: string; customizeValue?: (v: any) => any; customRow?: boolean; skip?: boolean };
}

type StatusPageState = {
  data: Array<{ [k: string]: string }>;
};

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

export const StatusContent: FC<StatusPageState> = ({ data }) => {
  return (
    <>
      {data.map((statuses, i) => {
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
      })}
    </>
  );
};

const Status: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => (
  <Fetch urls={endpoints.map(e => `${pathPrefix}${e}`)}>
    {({ error, data }) => {
      return (
        <StatusIndicator error={error && `Error fetching status: ${error.message}`} hasData={data && data.length}>
          <StatusContent data={data} />
        </StatusIndicator>
      );
    }}
  </Fetch>
);

export default Status;
