import { parseRange, parseTime, formatRange, formatTime } from './timeFormat';
import { PanelOptions, PanelType, PanelDefaultOptions } from '../Panel';
import { generateID } from './func';
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
      return range !== null ? { range } : {};

    case 'end_input':
    case 'moment_input':
      return { endTime: parseTime(decodedValue) };

    case 'step_input':
      const resolution = parseInt(decodedValue);
      return resolution > 0 ? { resolution } : {};
  }
  return {};
};

export const encodePanelOptionsToQueryString = (panels: PanelMeta[]) => {
  const queryParams: string[] = [];

  panels.forEach(({ key, options }) => {
    const { expr, type, stacked, range, endTime, resolution } = options;
    const panelParams: { [key: string]: string | undefined } = {
      expr: expr,
      tab: type === PanelType.Graph ? '0' : '1',
      stacked: stacked ? '1' : '0',
      range_input: formatRange(range),
      end_input: endTime !== null ? formatTime(endTime) : undefined,
      moment_input: endTime !== null ? formatTime(endTime) : undefined,
      step_input: resolution !== null ? resolution.toString() : undefined,
    };

    for (const o in panelParams) {
      const pp = panelParams[o];
      if (pp !== undefined) {
        queryParams.push(`g${key}.${o}=${encodeURIComponent(pp)}`);
      }
    }
  });

  return `?${queryParams.join('&')}`;
};
