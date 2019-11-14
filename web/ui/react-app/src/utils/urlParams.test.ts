import { decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString } from './urlParams';
import { PanelType } from '../Panel';

const panels = [
  {
    key: '0',
    options: {
      endTime: 1572046620000,
      expr: 'rate(node_cpu_seconds_total{mode="system"}[1m])',
      range: 3600,
      resolution: null,
      stacked: false,
      type: PanelType.Graph,
    },
  },
  {
    key: '1',
    options: {
      endTime: null,
      expr: 'node_filesystem_avail_bytes',
      range: 3600,
      resolution: null,
      stacked: false,
      type: PanelType.Table,
    },
  },
];
const query =
  '?g0.expr=rate(node_cpu_seconds_total%7Bmode%3D%22system%22%7D%5B1m%5D)&g0.tab=0&g0.stacked=0&g0.range_input=1h&g0.end_input=2019-10-25%2023%3A37&g0.moment_input=2019-10-25%2023%3A37&g1.expr=node_filesystem_avail_bytes&g1.tab=1&g1.stacked=0&g1.range_input=1h';

describe('decodePanelOptionsFromQueryString', () => {
  it('returns [] when query is empty', () => {
    expect(decodePanelOptionsFromQueryString('')).toEqual([]);
  });
  it('returns and array of parsed params when query string is non-empty', () => {
    expect(decodePanelOptionsFromQueryString(query)).toEqual(panels);
  });
});

describe('encodePanelOptionsToQueryString', () => {
  it('returns ? when panels is empty', () => {
    expect(encodePanelOptionsToQueryString([])).toEqual('?');
  });
  it('returns an encoded query string otherwise', () => {
    expect(encodePanelOptionsToQueryString(panels)).toEqual(query);
  });
});
