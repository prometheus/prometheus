import { decodePanelOptionsFromQueryString, encodePanelOptionsToQueryString, parseOption, toQueryString } from './urlParams';
import { PanelType } from '../Panel';
import moment from 'moment';

const panels: any = [
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
    expect(decodePanelOptionsFromQueryString(query)).toMatchObject(panels);
  });
});

describe('parseOption', () => {
  it('should return empty object for invalid param', () => {
    expect(parseOption('invalid_prop=foo')).toEqual({});
  });
  it('should parse expr param', () => {
    expect(parseOption('expr=foo')).toEqual({ expr: 'foo' });
  });
  it('should parse stacked', () => {
    expect(parseOption('stacked=1')).toEqual({ stacked: true });
  });
  it('should parse end_input', () => {
    expect(parseOption('end_input=2019-10-25%2023%3A37')).toEqual({ endTime: moment.utc('2019-10-25 23:37').valueOf() });
  });
  it('should parse moment_input', () => {
    expect(parseOption('moment_input=2019-10-25%2023%3A37')).toEqual({ endTime: moment.utc('2019-10-25 23:37').valueOf() });
  });
  describe('step_input', () => {
    it('should return step_input parsed if > 0', () => {
      expect(parseOption('step_input=2')).toEqual({ resolution: 2 });
    });
    it('should return empty object if step is equal 0', () => {
      expect(parseOption('step_input=0')).toEqual({});
    });
  });
  describe('range_input', () => {
    it('should return range parsed if its not null', () => {
      expect(parseOption('range_input=2h')).toEqual({ range: 7200 });
    });
    it('should return empty object for invalid value', () => {
      expect(parseOption('range_input=h')).toEqual({});
    });
  });
  describe('Parse type param', () => {
    it('should return panel type "graph" if tab=0', () => {
      expect(parseOption('tab=0')).toEqual({ type: PanelType.Graph });
    });
    it('should return panel type "table" if tab=1', () => {
      expect(parseOption('tab=1')).toEqual({ type: PanelType.Table });
    });
  });
});

describe('toQueryString', () => {
  it('should generate query string from panel options', () => {
    expect(
      toQueryString({
        id: 'asdf',
        key: '0',
        options: { expr: 'foo', type: PanelType.Graph, stacked: true, range: 0, endTime: null, resolution: 1 },
      })
    ).toEqual('g0.expr=foo&g0.tab=0&g0.stacked=1&g0.range_input=0y&g0.step_input=1');
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
