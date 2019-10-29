import { parseRange, parseTime, formatRange, formatTime } from './timeFormat';
import { PanelOptions, PanelType, PanelDefaultOptions } from '../Panel';

export function decodePanelOptionsFromQueryString(query: string): { key: string; options: PanelOptions }[] {
  if (query === '') {
    return [];
  }

  const params = query.substring(1).split('&');
  return parseParams(params);
}

const paramFormat = /^g\d+\..+=.+$/;

interface IncompletePanelOptions {
  expr?: string;
  type?: PanelType;
  range?: number;
  endTime?: number | null;
  resolution?: number | null;
  stacked?: boolean;
}

function parseParams(params: string[]): { key: string; options: PanelOptions }[] {
  const sortedParams = params
    .filter(p => {
      return paramFormat.test(p);
    })
    .sort();

  const panelOpts: { key: string; options: PanelOptions }[] = [];

  let key = 0;
  let options: IncompletePanelOptions = {};
  for (const p of sortedParams) {
    const prefix = 'g' + key + '.';

    if (!p.startsWith(prefix)) {
      panelOpts.push({
        key: key.toString(),
        options: { ...PanelDefaultOptions, ...options },
      });
      options = {};
      key++;
    }

    addParam(options, p.substring(prefix.length));
  }
  panelOpts.push({
    key: key.toString(),
    options: { ...PanelDefaultOptions, ...options },
  });

  return panelOpts;
}

function addParam(opts: IncompletePanelOptions, param: string): void {
  let [opt, val] = param.split('=');
  val = decodeURIComponent(val.replace(/\+/g, ' '));

  switch (opt) {
    case 'expr':
      opts.expr = val;
      break;

    case 'tab':
      if (val === '0') {
        opts.type = PanelType.Graph;
      } else {
        opts.type = PanelType.Table;
      }
      break;

    case 'stacked':
      opts.stacked = val === '1';
      break;

    case 'range_input':
      const range = parseRange(val);
      if (range !== null) {
        opts.range = range;
      }
      break;

    case 'end_input':
      opts.endTime = parseTime(val);
      break;

    case 'step_input':
      const res = parseInt(val);
      if (res > 0) {
        opts.resolution = res;
      }
      break;

    case 'moment_input':
      opts.endTime = parseTime(val);
      break;
  }
}

export function encodePanelOptionsToQueryString(panels: { key: string; options: PanelOptions }[]): string {
  const queryParams: string[] = [];

  panels.forEach(p => {
    const prefix = 'g' + p.key + '.';
    const o = p.options;
    const panelParams: { [key: string]: string | undefined } = {
      expr: o.expr,
      tab: o.type === PanelType.Graph ? '0' : '1',
      stacked: o.stacked ? '1' : '0',
      range_input: formatRange(o.range),
      end_input: o.endTime !== null ? formatTime(o.endTime) : undefined,
      moment_input: o.endTime !== null ? formatTime(o.endTime) : undefined,
      step_input: o.resolution !== null ? o.resolution.toString() : undefined,
    };

    for (const o in panelParams) {
      const pp = panelParams[o];
      if (pp !== undefined) {
        queryParams.push(prefix + o + '=' + encodeURIComponent(pp));
      }
    }
  });

  return '?' + queryParams.join('&');
}
