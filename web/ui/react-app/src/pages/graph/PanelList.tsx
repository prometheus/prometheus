import { FC, useEffect, useState } from 'react';
import { Alert, Button, Toast, ToastBody } from 'reactstrap';

import Checkbox from '../../components/Checkbox';
import { API_PATH } from '../../constants/constants';
import { ToastContext } from '../../contexts/ToastContext';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { useFetch } from '../../hooks/useFetch';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import { callAll, decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString, generateID } from '../../utils';
import Panel, { PanelDefaultOptions, PanelOptions } from './Panel';

export type PanelMeta = { key: string; options: PanelOptions; id: string };

export const updateURL = (nextPanels: PanelMeta[]): void => {
  const query = encodePanelOptionsToQueryString(nextPanels);
  window.history.pushState({}, '', query);
};

interface PanelListContentProps {
  panels: PanelMeta[];
  metrics: string[];
  useLocalTime: boolean;
  queryHistoryEnabled: boolean;
  enableAutocomplete: boolean;
  enableHighlighting: boolean;
  enableLinter: boolean;
}

export const PanelListContent: FC<PanelListContentProps> = ({
  metrics = [],
  useLocalTime,
  queryHistoryEnabled,
  enableAutocomplete,
  enableHighlighting,
  enableLinter,
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

  const pathPrefix = usePathPrefix();

  return (
    <>
      {panels.map(({ id, options }) => (
        <Panel
          pathPrefix={pathPrefix}
          onExecuteQuery={handleExecuteQuery}
          key={id}
          id={id}
          options={options}
          onOptionsChanged={(opts) =>
            callAll(setPanels, updateURL)(panels.map((p) => (id === p.id ? { ...p, options: opts } : p)))
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
          enableAutocomplete={enableAutocomplete}
          enableHighlighting={enableHighlighting}
          enableLinter={enableLinter}
        />
      ))}
      <Button className="d-block mb-3" color="primary" onClick={addPanel}>
        Add Panel
      </Button>
    </>
  );
};

const PanelList: FC = () => {
  const [delta, setDelta] = useState(0);
  const [useLocalTime, setUseLocalTime] = useLocalStorage('use-local-time', false);
  const [enableQueryHistory, setEnableQueryHistory] = useLocalStorage('enable-query-history', false);
  const [enableAutocomplete, setEnableAutocomplete] = useLocalStorage('enable-metric-autocomplete', true);
  const [enableHighlighting, setEnableHighlighting] = useLocalStorage('enable-syntax-highlighting', true);
  const [enableLinter, setEnableLinter] = useLocalStorage('enable-linter', true);
  const [clipboardMsg, setClipboardMsg] = useState<string | null>(null);

  const pathPrefix = usePathPrefix();
  const { response: metricsRes, error: metricsErr } = useFetch<string[]>(`${pathPrefix}/${API_PATH}/label/__name__/values`);

  const browserTime = new Date().getTime() / 1000;
  const { response: timeRes, error: timeErr } = useFetch<{ result: number[] }>(
    `${pathPrefix}/${API_PATH}/query?query=time()`
  );

  const onClipboardMsg = (msg: string) => {
    setClipboardMsg(msg);
    setTimeout(() => {
      setClipboardMsg(null);
    }, 1500);
  };

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
      <ToastContext.Provider value={onClipboardMsg}>
        <div className="clearfix">
          <Toast
            isOpen={clipboardMsg != null}
            style={{ position: 'fixed', zIndex: 1000, left: '50%', transform: 'translateX(-50%)' }}
          >
            <ToastBody>Label matcher copied to clipboard</ToastBody>
          </Toast>
          <div className="float-left">
            <Checkbox
              wrapperStyles={{ display: 'inline-block' }}
              id="use-local-time-checkbox"
              onChange={({ target }) => setUseLocalTime(target.checked)}
              defaultChecked={useLocalTime}
            >
              Use local time
            </Checkbox>
            <Checkbox
              wrapperStyles={{ marginLeft: 20, display: 'inline-block' }}
              id="query-history-checkbox"
              onChange={({ target }) => setEnableQueryHistory(target.checked)}
              defaultChecked={enableQueryHistory}
            >
              Enable query history
            </Checkbox>
            <Checkbox
              wrapperStyles={{ marginLeft: 20, display: 'inline-block' }}
              id="autocomplete-checkbox"
              onChange={({ target }) => setEnableAutocomplete(target.checked)}
              defaultChecked={enableAutocomplete}
            >
              Enable autocomplete
            </Checkbox>
          </div>
          <Checkbox
            wrapperStyles={{ marginLeft: 20, display: 'inline-block' }}
            id="highlighting-checkbox"
            onChange={({ target }) => setEnableHighlighting(target.checked)}
            defaultChecked={enableHighlighting}
          >
            Enable highlighting
          </Checkbox>
          <Checkbox
            wrapperStyles={{ marginLeft: 20, display: 'inline-block' }}
            id="linter-checkbox"
            onChange={({ target }) => setEnableLinter(target.checked)}
            defaultChecked={enableLinter}
          >
            Enable linter
          </Checkbox>
        </div>
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
          useLocalTime={useLocalTime}
          metrics={metricsRes.data}
          queryHistoryEnabled={enableQueryHistory}
          enableAutocomplete={enableAutocomplete}
          enableHighlighting={enableHighlighting}
          enableLinter={enableLinter}
        />
      </ToastContext.Provider>
    </>
  );
};

export default PanelList;
