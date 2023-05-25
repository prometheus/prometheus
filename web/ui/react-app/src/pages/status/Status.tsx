import React, { Fragment, FC, useState, useEffect } from 'react';
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

const StatusResult: FC<{ fetchPath: string; title: string }> = ({ fetchPath, title }) => {
  const { response, isLoading, error } = useFetch(fetchPath);
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
};

interface StatusProps {
  agentMode?: boolean | false;
  setAnimateLogo?: (animateLogo: boolean) => void;
}

const Status: FC<StatusProps> = ({ agentMode, setAnimateLogo }) => {
  /*    _
   *   /' \
   *  |    |
   *   \__/ */

  const [inputText, setInputText] = useState('');

  useEffect(() => {
    const handleKeyPress = (event: KeyboardEvent) => {
      const keyPressed = event.key.toUpperCase();
      setInputText((prevInputText) => {
        const newInputText = prevInputText.slice(-3) + String.fromCharCode(((keyPressed.charCodeAt(0) - 64) % 26) + 65);
        return newInputText;
      });
    };

    document.addEventListener('keypress', handleKeyPress);

    return () => {
      document.removeEventListener('keypress', handleKeyPress);
    };
  }, []);

  useEffect(() => {
    if (setAnimateLogo && inputText != '') {
      setAnimateLogo(inputText.toUpperCase() === 'QSPN');
    }
  }, [inputText]);

  /*    _
   *   /' \
   *  |    |
   *   \__/ */

  const pathPrefix = usePathPrefix();
  const path = `${pathPrefix}/${API_PATH}`;

  return (
    <>
      {[
        { fetchPath: `${path}/status/runtimeinfo`, title: 'Runtime Information' },
        { fetchPath: `${path}/status/buildinfo`, title: 'Build Information' },
        { fetchPath: `${path}/alertmanagers`, title: 'Alertmanagers' },
      ].map(({ fetchPath, title }) => {
        if (agentMode && title === 'Alertmanagers') {
          return null;
        }
        return <StatusResult fetchPath={fetchPath} title={title} />;
      })}
    </>
  );
};

export default Status;
