import React, { FC, useState, useEffect } from 'react';
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
    // We want useEffect to act only as componentDidMount, but react still complains about the empty dependencies list.
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
    callAll(
      setPanels,
      updateURL
    )([
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
            callAll(
              setPanels,
              updateURL
            )(
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
      <Button className="mb-3" color="primary" onClick={addPanel}>
        Add Panel
      </Button>
    </>
  );
};

const PanelList: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  const [delta, setDelta] = useState(0);
  const [useLocalTime, setUseLocalTime] = useLocalStorage('use-local-time', false);
  const [enableQueryHistory, setEnableQueryHistory] = useLocalStorage('enable-query-history', false);

  const { response: metricsRes, error: metricsErr } = useFetch<string[]>(`${pathPrefix}/api/v1/label/__name__/values`);

  const browserTime = new Date().getTime() / 1000;
  const { response: timeRes, error: timeErr } = useFetch<{ result: number[] }>(`${pathPrefix}/api/v1/query?query=time()`);

  useEffect(() => {
    if (timeRes.data) {
      const serverTime = timeRes.data.result[0];
      setDelta(Math.abs(browserTime - serverTime));
    }
    /**
     * React wants to include browserTime to useEffect dependencies list which will cause a delta change on every re-render
     * Basically it's not recommended to disable this rule, but this is the only way to take control over the useEffect
     * dependencies and to not include the browserTime variable.
     **/
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeRes.data]);

  return (
    <>
      <Checkbox
        wrapperStyles={{ marginLeft: 3, display: 'inline-block' }}
        id="query-history-checkbox"
        onChange={({ target }) => setEnableQueryHistory(target.checked)}
        defaultChecked={enableQueryHistory}
      >
        Enable query history
      </Checkbox>
      <Checkbox
        wrapperStyles={{ marginLeft: 20, display: 'inline-block' }}
        id="use-local-time-checkbox"
        onChange={({ target }) => setUseLocalTime(target.checked)}
        defaultChecked={useLocalTime}
      >
        Use local time
      </Checkbox>
      {(delta > 30 || timeErr) && (
        <Alert color="danger">
          <strong>Warning: </strong>
          {timeErr && `Unexpected response status when fetching server time: ${timeErr.message}`}
          {delta >= 30 &&
            `Error fetching server time: Detected ${delta} seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.`}
        </Alert>
      )}
      {metricsErr && (
        <Alert color="danger">
          <strong>Warning: </strong>
          Error fetching metrics list: Unexpected response status when fetching metric names: {metricsErr.message}
        </Alert>
      )}
      <PanelListContent
        panels={decodePanelOptionsFromQueryString(window.location.search)}
        pathPrefix={pathPrefix}
        useLocalTime={useLocalTime}
        metrics={metricsRes.data}
        queryHistoryEnabled={enableQueryHistory}
      />
    </>
  );
};

export default PanelList;
