import React, { Fragment, FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { withStatusIndicator } from '../withStatusIndicator';
import { useFetch } from '../utils/useFetch';
import PathPrefixProps from '../PathPrefixProps';

const sectionTitles = ['Runtime Information', 'Build Information', 'Alertmanagers'];

interface StatusConfig {
  [k: string]: { title?: string; customizeValue?: (v: any, key: string) => any; customRow?: boolean; skip?: boolean };
}

type StatusPageState = { [k: string]: string };

interface StatusPageProps {
  data?: StatusPageState[];
}

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
    customizeValue: (alertMgrs: { url: string }[], key) => {
      return (
        <Fragment key={key}>
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

export const StatusContent: FC<StatusPageProps> = ({ data = [] }) => {
  return (
    <>
      {data.map((statuses, i) => {
        return (
          <Fragment key={i}>
            <h2>{sectionTitles[i]}</h2>
            <Table className="h-auto" size="sm" bordered striped>
              <tbody>
                {Object.entries(statuses).map(([k, v], i) => {
                  const { title = k, customizeValue = (val: any) => val, customRow, skip } = statusConfig[k] || {};
                  if (skip) {
                    return null;
                  }
                  if (customRow) {
                    return customizeValue(v, k);
                  }
                  return (
                    <tr key={k}>
                      <th className="capitalize-title" style={{ width: '35%' }}>
                        {title}
                      </th>
                      <td className="text-break">{customizeValue(v, title)}</td>
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
const StatusWithStatusIndicator = withStatusIndicator(StatusContent);

StatusContent.displayName = 'Status';

const Status: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  const path = `${pathPrefix}/api/v1`;
  const status = useFetch<StatusPageState>(`${path}/status/runtimeinfo`);
  const runtime = useFetch<StatusPageState>(`${path}/status/buildinfo`);
  const build = useFetch<StatusPageState>(`${path}/alertmanagers`);

  let data;
  if (status.response.data && runtime.response.data && build.response.data) {
    data = [status.response.data, runtime.response.data, build.response.data];
  }

  return (
    <StatusWithStatusIndicator
      data={data}
      isLoading={status.isLoading || runtime.isLoading || build.isLoading}
      error={status.error || runtime.error || build.error}
    />
  );
};

export default Status;
