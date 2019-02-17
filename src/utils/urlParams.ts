import { parseRange, parseTime } from './timeFormat';
import { PanelOptions, PanelType, PanelDefaultOptions } from '../Panel';

export function getPanelOptionsFromQueryString(query: string): PanelOptions[] {
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

function parseParams(params: string[]): PanelOptions[] {
  const sortedParams = params.filter((p) => {
    return paramFormat.test(p);
  }).sort();

  let panelOpts: PanelOptions[] = [];

  let key = 0;
  let options: IncompletePanelOptions = {};
  for (const p of sortedParams) {
    const prefix = 'g' + key + '.';

    if (!p.startsWith(prefix)) {
      panelOpts.push({
        ...PanelDefaultOptions,
        ...options,
      });
      options = {};
      key++;
    }

    addParam(options, p.substring(prefix.length));
  }
  panelOpts.push({
    ...PanelDefaultOptions,
    ...options,
  });

  return panelOpts;
}

function addParam(opts: IncompletePanelOptions, param: string): void {
  let [ opt, val ] = param.split('=');
  val = decodeURIComponent(val.replace(/\+/g, ' '));
  console.log(val);

  switch(opt) {
    case 'expr':
    console.log(val);
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
      const res = parseInt(val)
      if (res > 0) {
        opts.resolution = res;
      }
      break;

    case 'moment_input':
      opts.endTime = parseTime(val);
      break;
  }
}
