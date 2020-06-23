import moment from 'moment';

import {
  escapeHTML,
  metricToSeriesName,
  formatTime,
  parseTime,
  formatRange,
  parseRange,
  humanizeDuration,
  formatRelative,
  now,
  toQueryString,
  encodePanelOptionsToQueryString,
  parseOption,
  decodePanelOptionsFromQueryString,
  parsePrometheusFloat,
} from '.';
import { PanelType } from '../pages/graph/Panel';

describe('Utils', () => {
  describe('escapeHTML', (): void => {
    it('escapes html sequences', () => {
      expect(escapeHTML(`<strong>'example'&"another/example"</strong>`)).toEqual(
        '&lt;strong&gt;&#39;example&#39;&amp;&quot;another&#x2F;example&quot;&lt;&#x2F;strong&gt;'
      );
    });
  });

  describe('metricToSeriesName', () => {
    it('returns "{}" if labels is empty', () => {
      const labels = {};
      expect(metricToSeriesName(labels)).toEqual('{}');
    });
    it('returns "metric_name{}" if labels only contains __name__', () => {
      const labels = { __name__: 'metric_name' };
      expect(metricToSeriesName(labels)).toEqual('metric_name{}');
    });
    it('returns "{label1=value_1, ..., labeln=value_n} if there are many labels and no name', () => {
      const labels = { label1: 'value_1', label2: 'value_2', label3: 'value_3' };
      expect(metricToSeriesName(labels)).toEqual('{label1="value_1", label2="value_2", label3="value_3"}');
    });
    it('returns "metric_name{label1=value_1, ... ,labeln=value_n}" if there are many labels and a name', () => {
      const labels = {
        __name__: 'metric_name',
        label1: 'value_1',
        label2: 'value_2',
        label3: 'value_3',
      };
      expect(metricToSeriesName(labels)).toEqual('metric_name{label1="value_1", label2="value_2", label3="value_3"}');
    });
  });

  describe('Time format', () => {
    describe('formatTime', () => {
      it('returns a time string representing the time in seconds', () => {
        expect(formatTime(1572049380000)).toEqual('2019-10-26 00:23:00');
        expect(formatTime(0)).toEqual('1970-01-01 00:00:00');
      });
    });

    describe('parseTime', () => {
      it('returns a time string representing the time in seconds', () => {
        expect(parseTime('2019-10-26 00:23')).toEqual(1572049380000);
        expect(parseTime('1970-01-01 00:00')).toEqual(0);
        expect(parseTime('0001-01-01T00:00:00Z')).toEqual(-62135596800000);
      });
    });

    describe('formatRange', () => {
      it('returns a time string representing the time in seconds in one unit', () => {
        expect(formatRange(60 * 60 * 24 * 365)).toEqual('1y');
        expect(formatRange(60 * 60 * 24 * 7)).toEqual('1w');
        expect(formatRange(2 * 60 * 60 * 24)).toEqual('2d');
        expect(formatRange(60 * 60)).toEqual('1h');
        expect(formatRange(7 * 60)).toEqual('7m');
        expect(formatRange(63)).toEqual('63s');
      });
    });

    describe('parseRange', () => {
      it('returns a time string representing the time in seconds in one unit', () => {
        expect(parseRange('1y')).toEqual(60 * 60 * 24 * 365);
        expect(parseRange('1w')).toEqual(60 * 60 * 24 * 7);
        expect(parseRange('2d')).toEqual(2 * 60 * 60 * 24);
        expect(parseRange('1h')).toEqual(60 * 60);
        expect(parseRange('7m')).toEqual(7 * 60);
        expect(parseRange('63s')).toEqual(63);
      });
    });

    describe('humanizeDuration', () => {
      it('humanizes zero', () => {
        expect(humanizeDuration(0)).toEqual('0s');
      });
      it('humanizes milliseconds', () => {
        expect(humanizeDuration(1.234567)).toEqual('1.235ms');
        expect(humanizeDuration(12.34567)).toEqual('12.346ms');
        expect(humanizeDuration(123.45678)).toEqual('123.457ms');
        expect(humanizeDuration(123)).toEqual('123.000ms');
      });
      it('humanizes seconds', () => {
        expect(humanizeDuration(12340)).toEqual('12.340s');
      });
      it('humanizes minutes', () => {
        expect(humanizeDuration(1234567)).toEqual('20m 34s');
      });

      it('humanizes hours', () => {
        expect(humanizeDuration(12345678)).toEqual('3h 25m 45s');
      });

      it('humanizes days', () => {
        expect(humanizeDuration(123456789)).toEqual('1d 10h 17m 36s');
        expect(humanizeDuration(123456789000)).toEqual('1428d 21h 33m 9s');
      });
      it('takes sign into account', () => {
        expect(humanizeDuration(-123456789000)).toEqual('-1428d 21h 33m 9s');
      });
    });

    describe('formatRelative', () => {
      it('renders never for pre-beginning-of-time strings', () => {
        expect(formatRelative('0001-01-01T00:00:00Z', now())).toEqual('Never');
      });
      it('renders a humanized duration for sane durations', () => {
        expect(formatRelative('2019-11-04T09:15:29.578701-07:00', parseTime('2019-11-04T09:15:35.8701-07:00'))).toEqual(
          '6.292s'
        );
        expect(formatRelative('2019-11-04T09:15:35.8701-07:00', parseTime('2019-11-04T09:15:29.578701-07:00'))).toEqual(
          '-6.292s'
        );
      });
    });
  });

  describe('URL Params', () => {
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
      '?g0.expr=rate(node_cpu_seconds_total%7Bmode%3D%22system%22%7D%5B1m%5D)&g0.tab=0&g0.stacked=0&g0.range_input=1h&g0.end_input=2019-10-25%2023%3A37%3A00&g0.moment_input=2019-10-25%2023%3A37%3A00&g1.expr=node_filesystem_avail_bytes&g1.tab=1&g1.stacked=0&g1.range_input=1h';

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
        expect(parseOption('moment_input=2019-10-25%2023%3A37')).toEqual({
          endTime: moment.utc('2019-10-25 23:37').valueOf(),
        });
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

    describe('parsePrometheusFloat', () => {
      it('returns Inf when param is Inf', () => {
        expect(parsePrometheusFloat('Inf')).toEqual('Inf');
      });
      it('returns +Inf when param is +Inf', () => {
        expect(parsePrometheusFloat('+Inf')).toEqual('+Inf');
      });
      it('returns -Inf when param is -Inf', () => {
        expect(parsePrometheusFloat('-Inf')).toEqual('-Inf');
      });
      it('returns 17 when param is 1.7e+01', () => {
        expect(parsePrometheusFloat('1.7e+01')).toEqual(17);
      });
      it('returns -17 when param is -1.7e+01', () => {
        expect(parsePrometheusFloat('-1.7e+01')).toEqual(-17);
      });
    });
  });
});
