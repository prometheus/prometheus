import { parseRange, parseTime, formatRange, formatTime } from './timeFormat';
import { PanelOptions, PanelType, PanelDefaultOptions } from '../Panel';
import { generateID } from './func';
import { PanelMeta } from '../pages/PanelList';

export const decodePanelOptionsFromQueryString = (query: string): PanelMeta[] => {
  return query === '' ? [] : parseParams(query.substring(1).split('&'));
};

const byParamFormat = (p: string) => /^g\d+\..+=.+$/.test(p);

const parseParams = (params: string[]) => {
  let key = 0;
  return params
    .filter(byParamFormat)
    .sort()
    .reduce<PanelMeta[]>((panels, urlParam, i, sortedParams) => {
      const prefix = `g${key}.`;

      if (urlParam.startsWith(`${prefix}expr=`)) {
        let options: Partial<PanelOptions> = {};

        for (let index = i; index < sortedParams.length; index++) {
          const param = sortedParams[index];
          if (!param.startsWith(prefix)) {
            break;
          }
          options = { ...options, ...parseOption(param.substring(prefix.length)) };
        }

        return [
          ...panels,
          {
            id: generateID(),
            key: `${key++}`,
            options: { ...PanelDefaultOptions, ...options },
          },
        ];
      }

      return panels;
    }, []);
};

const parseOption = (param: string): Partial<PanelOptions> => {
  let [opt, val] = param.split('=');
  val = decodeURIComponent(val.replace(/\+/g, ' '));
  switch (opt) {
    case 'expr':
      return { expr: val };

    case 'tab':
      return { type: val === '0' ? PanelType.Graph : PanelType.Table };

    case 'stacked':
      return { stacked: val === '1' };

    case 'range_input':
      const range = parseRange(val);
      return range ? { range } : {};

    case 'end_input':
    case 'moment_input':
      return { endTime: parseTime(val) };

    case 'step_input':
      const resolution = parseInt(val);
      return resolution ? { resolution } : {};
  }
  return {};
};

export const encodePanelOptionsToQueryString = (panels: PanelMeta[]) => {
  const queryParams: string[] = [];

  panels.forEach(({ key, options }) => {
    const prefix = `g${key}.`;
    const { expr, type, stacked, range, endTime, resolution } = options;
    const panelParams: { [key: string]: string | undefined } = {
      expr,
      tab: type === PanelType.Graph ? '0' : '1',
      stacked: stacked ? '1' : '0',
      range_input: formatRange(range),
      end_input: endTime ? formatTime(endTime) : undefined,
      moment_input: endTime ? formatTime(endTime) : undefined,
      step_input: resolution ? resolution.toString() : undefined,
    };

    for (const o in panelParams) {
      const pp = panelParams[o];
      if (pp !== undefined) {
        queryParams.push(prefix + o + '=' + encodeURIComponent(pp));
      }
    }
  });

  return '?' + queryParams.join('&');
};
