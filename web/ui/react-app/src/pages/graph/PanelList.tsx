import React, { FC, useState, useEffect, useCallback } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Button } from 'reactstrap';

import Panel, { PanelOptions, PanelDefaultOptions } from './Panel';
import Checkbox from '../../components/Checkbox';
import PathPrefixProps from '../../types/PathPrefixProps';
import { generateID, decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString, callAll } from '../../utils';
import { useFetch } from '../../hooks/useFetch';
import { useLocalStorage } from '../../hooks/useLocalStorage';

export type PanelMeta = { key: string; options: PanelOptions; id: string };

export const updateURL = (nextPanels: PanelMeta[]) => {
  const query = encodePanelOptionsToQueryString(nextPanels);
  window.history.pushState({}, '', query);
};

interface PanelListProps extends PathPrefixProps, RouteComponentProps {
  panels: PanelMeta[];
  metrics: string[];
  useLocalTime: boolean;
  queryHistoryEnabled: boolean;
}

export const PanelListContent: FC<PanelListProps> = ({
  metrics = [],
  useLocalTime,
  pathPrefix,
  queryHistoryEnabled,
  ...rest
}) => {
  const [panels, setPanels] = useState(rest.panels);
  const [historyItems, setLocalStorageHistoryItems] = useLocalStorage<string[]>('history', []);

  useEffect(() => {
    !panels.length && addPanel();
    window.onpopstate = () => {
      const panels = decodePanelOptionsFromQueryString(window.location.search);
      if (panels.length > 0) {
        setPanels(panels);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleExecuteQuery = (query: string) => {
    const isSimpleMetric = metrics.indexOf(query) !== -1;
    if (isSimpleMetric || !query.length) {
      return;
    }
    const extendedItems = historyItems.reduce(
      (acc, metric) => {
        return metric === query ? acc : [...acc, metric]; // Prevent adding query twice.
      },
      [query]
    );
    setLocalStorageHistoryItems(extendedItems.slice(0, 50));
  };

  const addPanel = () => {
    callAll(setPanels, updateURL)([
      ...panels,
      {
        id: generateID(),
        key: `${panels.length}`,
        options: PanelDefaultOptions,
      },
    ]);
  };

  return (
    <>
      {panels.map(({ id, options }) => (
        <Panel
          onExecuteQuery={handleExecuteQuery}
          key={id}
          options={options}
          onOptionsChanged={opts =>
            callAll(setPanels, updateURL)(panels.map(p => (id === p.id ? { ...p, options: opts } : p)))
          }
          removePanel={() =>
            callAll(setPanels, updateURL)(
              panels.reduce<PanelMeta[]>(
                (acc, panel) => (panel.id !== id ? [...acc, { ...panel, key: `${acc.length}` }] : acc),
                []
              )
            )
          }
          useLocalTime={useLocalTime}
          metricNames={metrics}
          pastQueries={queryHistoryEnabled ? historyItems : []}
          pathPrefix={pathPrefix}
        />
      ))}
      <Button color="primary" onClick={addPanel}>
        Add Panel
      </Button>
    </>
  );
};

const opts: RequestInit = {
  cache: 'no-store',
};

const PanelList: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  const memoizedFetch = useCallback(useFetch, [pathPrefix]);
  const [delta, setDelta] = useState(0);
  const [useLocalTime, setUseLocalTime] = useLocalStorage('use-local-time', false);
  const [enableQueryHistory, setEnableQueryHistory] = useLocalStorage('enable-query-history', false);

  const { response: metricsResponse, error: metricsErr } = memoizedFetch<string[]>(
    `${pathPrefix}/api/v1/label/__name__/values`,
    opts
  );

  const browserTime = new Date().getTime() / 1000;
  const { response: timeResponse, error: timeError } = memoizedFetch<{ result: number[] }>(
    `${pathPrefix}/api/v1/query?query=time()`,
    opts
  );
  useEffect(() => {
    if (timeResponse.data) {
      const serverTime = timeResponse.data.result[0];
      setDelta(Math.abs(browserTime - serverTime));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeResponse.data]);

  return (
    <>
      <Checkbox
        id="query-history-checkbox"
        onChange={({ target }) => setEnableQueryHistory(target.checked)}
        defaultChecked={enableQueryHistory}
      >
        Enable query history
      </Checkbox>
      <Checkbox
        id="use-local-time-checkbox"
        onChange={({ target }) => setUseLocalTime(target.checked)}
        defaultChecked={useLocalTime}
      >
        Use local time
      </Checkbox>
      {(delta > 30 || timeError) && (
        <Alert color="danger">
          <strong>Warning: </strong>
          {timeError && `Unexpected response status when fetching server time: ${timeError.message}`}
          {delta > 30 &&
            `Error fetching server time: Detected ${delta} seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.`}
        </Alert>
      )}
      {metricsErr && (
        <Alert color="danger">
          <strong>Warning: </strong>
          {`Error fetching metrics list: Unexpected response status when fetching metric names: ${metricsErr.message}`}
        </Alert>
      )}
      <PanelListContent
        panels={decodePanelOptionsFromQueryString(window.location.search)}
        pathPrefix={pathPrefix}
        useLocalTime={useLocalTime}
        metrics={metricsResponse.data}
        queryHistoryEnabled={enableQueryHistory}
      />
    </>
  );
};

export default PanelList;
