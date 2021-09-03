import React, { Fragment, FC } from 'react';
import { Table } from 'reactstrap';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { useFetch } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';

interface StatusPageProps {
  data: Record<string, string>;
  title: string;
}

export const statusConfig: Record<
  string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  { title?: string; customizeValue?: (v: any, key: string) => any; customRow?: boolean; skip?: boolean }
> = {
  startTime: { title: 'Start time', customizeValue: (v: string) => new Date(v).toUTCString() },
  CWD: { title: 'Working directory' },
  reloadConfigSuccess: {
    title: 'Configuration reload',
    customizeValue: (v: boolean) => (v ? 'Successful' : 'Unsuccessful'),
  },
  lastConfigTime: { title: 'Last successful configuration reload' },
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

export const StatusContent: FC<StatusPageProps> = ({ data, title }) => {
  return (
    <>
      <h2>{title}</h2>
      <Table className="h-auto" size="sm" bordered striped>
        <tbody>
          {Object.entries(data).map(([k, v]) => {
            const { title = k, customizeValue = (val: string) => val, customRow, skip } = statusConfig[k] || {};
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
    </>
  );
};
const StatusWithStatusIndicator = withStatusIndicator(StatusContent);

StatusContent.displayName = 'Status';

const Status: FC = () => {
  const pathPrefix = usePathPrefix();
  const path = `${pathPrefix}/${API_PATH}`;

  return (
    <>
      {[
        { fetchResult: useFetch<Record<string, string>>(`${path}/status/runtimeinfo`), title: 'Runtime Information' },
        { fetchResult: useFetch<Record<string, string>>(`${path}/status/buildinfo`), title: 'Build Information' },
        { fetchResult: useFetch<Record<string, string>>(`${path}/alertmanagers`), title: 'Alertmanagers' },
      ].map(({ fetchResult, title }) => {
        const { response, isLoading, error } = fetchResult;
        return (
          <StatusWithStatusIndicator
            key={title}
            data={response.data}
            title={title}
            isLoading={isLoading}
            error={error}
            componentTitle={title}
          />
        );
      })}
    </>
  );
};

export default Status;
