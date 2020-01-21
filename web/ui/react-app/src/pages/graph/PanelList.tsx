import React, { ChangeEvent, FC, useState, useEffect } from 'react';
import { RouteComponentProps } from '@reach/router';

import { Alert, Button } from 'reactstrap';

import Panel, { PanelOptions, PanelDefaultOptions } from './Panel';
import Checkbox from '../../components/Checkbox';
import PathPrefixProps from '../../types/PathPrefixProps';
import { generateID, decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString, callAll } from '../../utils';
import { withStatusIndicator } from '../../components/withStatusIndicator';

export type PanelMeta = { key: string; options: PanelOptions; id: string };

export const isHistoryEnabled: () => boolean = () => JSON.parse(localStorage.getItem('enable-query-history') || 'false');

export const getHistoryItems: () => string[] = () => JSON.parse(localStorage.getItem('history') || '[]');

export const updateURL = (nextPanels: PanelMeta[]) => {
  const query = encodePanelOptionsToQueryString(nextPanels);
  window.history.pushState({}, '', query);
};

interface PanelListProps extends PathPrefixProps, RouteComponentProps {
  panels: PanelMeta[];
  metrics: string[];
  onExecuteQuery: () => void;
  pastQueries: string[];
  useLocalTime: boolean;
}

export const PanelListContent: FC<PanelListProps> = ({
  metrics = [],
  useLocalTime,
  pathPrefix,
  pastQueries,
  onExecuteQuery,
  ...rest
}) => {
  const [panels, setPanels] = useState(rest.panels);

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
    const historyItems = getHistoryItems();
    const extendedItems = historyItems.reduce(
      (acc, metric) => {
        return metric === query ? acc : [...acc, metric]; // Prevent adding query twice.
      },
      [query]
    );
    localStorage.setItem('history', JSON.stringify(extendedItems.slice(0, 50)));
    onExecuteQuery();
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
          pastQueries={pastQueries}
          pathPrefix={pathPrefix}
        />
      ))}
      <Button color="primary" onClick={addPanel}>
        Add Panel
      </Button>
    </>
  );
};

const PanelsWithStatus = withStatusIndicator<PanelListProps>(PanelListContent);

const PanelList: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const [metricNames, setMetricNames] = useState<string[]>([]);
  const [pastQueries, setPastQueries] = useState<string[]>([]);
  const [timeDriftError, setTimeDriftError] = useState();
  const [fetchMetricsError, setFetchMetricsError] = useState();
  const [useLocalTime, setUseLocalTime] = useState();

  useEffect(() => {
    (async () => {
      const metricsResponse = await fetch(`${pathPrefix}/api/v1/label/__name__/values`, { cache: 'no-store' });
      if (metricsResponse.ok) {
        const json = await metricsResponse.json();
        setMetricNames(json.data);
      } else {
        setFetchMetricsError(
          `Error fetching metrics list: Unexpected response status when fetching metric names: ${metricsResponse.statusText}`
        );
      }

      const browserTime = new Date().getTime() / 1000;
      const timeResponse = await fetch(`${pathPrefix}/api/v1/query?query=time()`, {
        cache: 'no-store',
      });
      if (timeResponse.ok) {
        const json = await timeResponse.json();
        const serverTime = json.data.result[0];
        const delta = Math.abs(browserTime - serverTime);
        if (delta >= 30) {
          setTimeDriftError(
            `Error fetching server time: Detected ${delta} seconds time difference between your browser and the server. Prometheus relies on accurate time and time drift might cause unexpected query results.`
          );
        }
      } else {
        setTimeDriftError('Unexpected response status when fetching server time:' + timeResponse.statusText);
      }
      updatePastQueries();
    })();
  }, [pathPrefix]);

  const toggleQueryHistory = (e: ChangeEvent<HTMLInputElement>) => {
    localStorage.setItem('enable-query-history', `${e.target.checked}`);
    updatePastQueries();
  };

  const updatePastQueries = () => {
    setPastQueries(isHistoryEnabled() ? getHistoryItems() : []);
  };

  const toggleUseLocalTime = (e: ChangeEvent<HTMLInputElement>) => {
    localStorage.setItem('use-local-time', `${e.target.checked}`);
    setUseLocalTime(e.target.checked);
  };

  return (
    <>
      <Checkbox id="history-checkbox" onChange={toggleQueryHistory} defaultChecked={isHistoryEnabled()}>
        Enable query history
      </Checkbox>
      <Checkbox
        id="use-local-time-checkbox"
        onChange={toggleUseLocalTime}
        defaultChecked={JSON.parse(localStorage.getItem('use-local-time') || 'false')}
      >
        Use local time
      </Checkbox>
      {timeDriftError && (
        <Alert color="danger">
          <strong>Warning:</strong> {timeDriftError}
        </Alert>
      )}
      {fetchMetricsError && (
        <Alert color="danger">
          <strong>Warning:</strong> {fetchMetricsError}
        </Alert>
      )}
      <PanelsWithStatus
        isLoading={!timeDriftError && !fetchMetricsError && metricNames.length === 0}
        panels={decodePanelOptionsFromQueryString(window.location.search)}
        pathPrefix={pathPrefix}
        useLocalTime={useLocalTime}
        metrics={metricNames}
        onExecuteQuery={updatePastQueries}
        pastQueries={pastQueries}
      />
    </>
  );
};

export default PanelList;
