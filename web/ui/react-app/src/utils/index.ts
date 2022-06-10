import moment from 'moment-timezone';

import { PanelOptions, PanelType, PanelDefaultOptions } from '../pages/graph/Panel';
import { PanelMeta } from '../pages/graph/PanelList';

export const generateID = (): string => {
  return `_${Math.random().toString(36).substr(2, 9)}`;
};

export const byEmptyString = (p: string): boolean => p.length > 0;

export const isPresent = <T>(obj: T): obj is NonNullable<T> => obj !== null && obj !== undefined;

export const escapeHTML = (str: string): string => {
  const entityMap: { [key: string]: string } = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
    '/': '&#x2F;',
  };

  return String(str).replace(/[&<>"'/]/g, function (s) {
    return entityMap[s];
  });
};

export const metricToSeriesName = (labels: { [key: string]: string }): string => {
  if (labels === null) {
    return 'scalar';
  }
  let tsName = (labels.__name__ || '') + '{';
  const labelStrings: string[] = [];
  for (const label in labels) {
    if (label !== '__name__') {
      labelStrings.push(label + '="' + labels[label] + '"');
    }
  }
  tsName += labelStrings.join(', ') + '}';
  return tsName;
};

export const parseDuration = (durationStr: string): number | null => {
  if (durationStr === '') {
    return null;
  }
  if (durationStr === '0') {
    // Allow 0 without a unit.
    return 0;
  }

  const durationRE = new RegExp('^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$');
  const matches = durationStr.match(durationRE);
  if (!matches) {
    return null;
  }

  let dur = 0;

  // Parse the match at pos `pos` in the regex and use `mult` to turn that
  // into ms, then add that value to the total parsed duration.
  const m = (pos: number, mult: number) => {
    if (matches[pos] === undefined) {
      return;
    }
    const n = parseInt(matches[pos]);
    dur += n * mult;
  };

  m(2, 1000 * 60 * 60 * 24 * 365); // y
  m(4, 1000 * 60 * 60 * 24 * 7); // w
  m(6, 1000 * 60 * 60 * 24); // d
  m(8, 1000 * 60 * 60); // h
  m(10, 1000 * 60); // m
  m(12, 1000); // s
  m(14, 1); // ms

  return dur;
};

export const formatDuration = (d: number): string => {
  let ms = d;
  let r = '';
  if (ms === 0) {
    return '0s';
  }

  const f = (unit: string, mult: number, exact: boolean) => {
    if (exact && ms % mult !== 0) {
      return;
    }
    const v = Math.floor(ms / mult);
    if (v > 0) {
      r += `${v}${unit}`;
      ms -= v * mult;
    }
  };

  // Only format years and weeks if the remainder is zero, as it is often
  // easier to read 90d than 12w6d.
  f('y', 1000 * 60 * 60 * 24 * 365, true);
  f('w', 1000 * 60 * 60 * 24 * 7, true);

  f('d', 1000 * 60 * 60 * 24, false);
  f('h', 1000 * 60 * 60, false);
  f('m', 1000 * 60, false);
  f('s', 1000, false);
  f('ms', 1, false);

  return r;
};

export function parseTime(timeText: string): number {
  return moment.utc(timeText).valueOf();
}

export function formatTime(time: number): string {
  return moment.utc(time).format('YYYY-MM-DD HH:mm:ss');
}

export const now = (): number => moment().valueOf();

export const humanizeDuration = (milliseconds: number): string => {
  const sign = milliseconds < 0 ? '-' : '';
  const unsignedMillis = milliseconds < 0 ? -1 * milliseconds : milliseconds;
  const duration = moment.duration(unsignedMillis, 'ms');
  const ms = Math.floor(duration.milliseconds());
  const s = Math.floor(duration.seconds());
  const m = Math.floor(duration.minutes());
  const h = Math.floor(duration.hours());
  const d = Math.floor(duration.asDays());
  if (d !== 0) {
    return `${sign}${d}d ${h}h ${m}m ${s}s`;
  }
  if (h !== 0) {
    return `${sign}${h}h ${m}m ${s}s`;
  }
  if (m !== 0) {
    return `${sign}${m}m ${s}s`;
  }
  if (s !== 0) {
    return `${sign}${s}.${ms}s`;
  }
  if (unsignedMillis > 0) {
    return `${sign}${unsignedMillis.toFixed(3)}ms`;
  }
  return '0s';
};

export const formatRelative = (startStr: string, end: number): string => {
  const start = parseTime(startStr);
  if (start < 0) {
    return 'Never';
  }
  return humanizeDuration(end - start) + ' ago';
};

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

    case 'show_exemplars':
      return { showExemplars: decodedValue === '1' };

    case 'range_input':
      const range = parseDuration(decodedValue);
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

export const formatParam =
  (key: string) =>
  (paramName: string, value: number | string | boolean): string => {
    return `g${key}.${paramName}=${encodeURIComponent(value)}`;
  };

export const toQueryString = ({ key, options }: PanelMeta): string => {
  const formatWithKey = formatParam(key);
  const { expr, type, stacked, range, endTime, resolution, showExemplars } = options;
  const time = isPresent(endTime) ? formatTime(endTime) : false;
  const urlParams = [
    formatWithKey('expr', expr),
    formatWithKey('tab', type === PanelType.Graph ? 0 : 1),
    formatWithKey('stacked', stacked ? 1 : 0),
    formatWithKey('show_exemplars', showExemplars ? 1 : 0),
    formatWithKey('range_input', formatDuration(range)),
    time ? `${formatWithKey('end_input', time)}&${formatWithKey('moment_input', time)}` : '',
    isPresent(resolution) ? formatWithKey('step_input', resolution) : '',
  ];
  return urlParams.filter(byEmptyString).join('&');
};

export const encodePanelOptionsToQueryString = (panels: PanelMeta[]): string => {
  return `?${panels.map(toQueryString).join('&')}`;
};

export const setQuerySearchFilter = (search: string) => {
  window.history.pushState({}, '', `?search=${search}`);
};

export const getQuerySearchFilter = (): string => {
  const locationSearch = window.location.search;
  const params = new URLSearchParams(locationSearch);
  const search = params.get('search') || '';
  return search;
};

export const createExpressionLink = (expr: string): string => {
  return `../graph?g0.expr=${encodeURIComponent(expr)}&g0.tab=1&g0.stacked=0&g0.show_exemplars=0.g0.range_input=1h.`;
};

export const mapObjEntries = <T, key extends keyof T, Z>(
  o: T,
  cb: ([k, v]: [string, T[key]], i: number, arr: [string, T[key]][]) => Z
): Z[] => Object.entries(o).map(cb);

export const callAll =
  (
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ...fns: Array<(...args: any) => void>
  ) =>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any, @typescript-eslint/explicit-module-boundary-types
  (...args: any): void => {
    // eslint-disable-next-line prefer-spread
    fns.filter(Boolean).forEach((fn) => fn.apply(null, args));
  };

export const parsePrometheusFloat = (value: string): string | number => {
  if (isNaN(Number(value))) {
    return value;
  } else {
    return Number(value);
  }
};

export function debounce<Params extends unknown[]>(
  func: (...args: Params) => unknown,
  timeout: number
): (...args: Params) => void {
  let timer: NodeJS.Timeout;
  return (...args: Params) => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      func(...args);
    }, timeout);
  };
}
