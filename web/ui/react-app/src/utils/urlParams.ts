import { parseRange, parseTime, formatRange, formatTime } from './timeFormat';
import { PanelOptions, PanelType, PanelDefaultOptions } from '../Panel';
import { generateID, byEmptyString, isPresent } from './func';
import { PanelMeta } from '../pages/PanelList';

const paramFormat = /^g\d+\..+=.+$/;

export const decodePanelOptionsFromQueryString = (query: string): PanelMeta[] => {
  if (query === '') {
    return [];
  }
  const urlParams = query.substring(1).split('&');

  return urlParams.reduce<PanelMeta[]>((panels, urlParam, i) => {
    const panelsCount = panels.length;
    const prefix = `g${panelsCount}.`;
    if (urlParam.startsWith(`${prefix}expr=`)) {
      const prefixLen = prefix.length;
      return [
        ...panels,
        {
          id: generateID(),
          key: `${panelsCount}`,
          options: urlParams.slice(i).reduce((opts, param) => {
            return param.startsWith(prefix) && paramFormat.test(param)
              ? { ...opts, ...parseOption(param.substring(prefixLen)) }
              : opts;
          }, PanelDefaultOptions),
        },
      ];
    }
    return panels;
  }, []);
};

export const parseOption = (param: string): Partial<PanelOptions> => {
  const [opt, val] = param.split('=');
  const decodedValue = decodeURIComponent(val.replace(/\+/g, ' '));
  switch (opt) {
    case 'expr':
      return { expr: decodedValue };

    case 'tab':
      return { type: decodedValue === '0' ? PanelType.Graph : PanelType.Table };

    case 'stacked':
      return { stacked: decodedValue === '1' };

    case 'range_input':
      const range = parseRange(decodedValue);
      return isPresent(range) ? { range } : {};

    case 'end_input':
    case 'moment_input':
      return { endTime: parseTime(decodedValue) };

    case 'step_input':
      const resolution = parseInt(decodedValue);
      return resolution > 0 ? { resolution } : {};
  }
  return {};
};

export const formatParam = (key: string) => (paramName: string, value: number | string | boolean) => {
  return `g${key}.${paramName}=${encodeURIComponent(value)}`;
};

export const toQueryString = ({ key, options }: PanelMeta) => {
  const formatWithKey = formatParam(key);
  const { expr, type, stacked, range, endTime, resolution } = options;
  const time = isPresent(endTime) ? formatTime(endTime) : false;
  const urlParams = [
    formatWithKey('expr', expr),
    formatWithKey('tab', type === PanelType.Graph ? 0 : 1),
    formatWithKey('stacked', stacked ? 1 : 0),
    formatWithKey('range_input', formatRange(range)),
    time ? `${formatWithKey('end_input', time)}&${formatWithKey('moment_input', time)}` : '',
    isPresent(resolution) ? formatWithKey('step_input', resolution) : '',
  ];
  return urlParams.filter(byEmptyString).join('&');
};

export const encodePanelOptionsToQueryString = (panels: PanelMeta[]) => {
  return `?${panels.map(toQueryString).join('&')}`;
};
