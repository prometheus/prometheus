import * as React from 'react';
import { mount, shallow, ReactWrapper } from 'enzyme';
import { act } from 'react-dom/test-utils';
import Flags, { FlagMap } from './Flags';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

const sampleFlagsResponse: {
  status: string;
  data: FlagMap;
} = {
  status: 'success',
  data: {
    'alertmanager.notification-queue-capacity': '10000',
    'alertmanager.timeout': '10s',
    'config.file': './documentation/examples/prometheus.yml',
    'log.format': 'logfmt',
    'log.level': 'info',
    'query.lookback-delta': '5m',
    'query.max-concurrency': '20',
    'query.max-samples': '50000000',
    'query.timeout': '2m',
    'rules.alert.for-grace-period': '10m',
    'rules.alert.for-outage-tolerance': '1h',
    'rules.alert.resend-delay': '1m',
    'storage.remote.flush-deadline': '1m',
    'storage.remote.read-concurrent-limit': '10',
    'storage.remote.read-max-bytes-in-frame': '1048576',
    'storage.remote.read-sample-limit': '50000000',
    'storage.tsdb.allow-overlapping-blocks': 'false',
    'storage.tsdb.max-block-duration': '36h',
    'storage.tsdb.min-block-duration': '2h',
    'storage.tsdb.no-lockfile': 'false',
    'storage.tsdb.path': 'data/',
    'storage.tsdb.retention': '0s',
    'storage.tsdb.retention.size': '0B',
    'storage.tsdb.retention.time': '0s',
    'storage.tsdb.wal-compression': 'false',
    'storage.tsdb.wal-segment-size': '0B',
    'web.console.libraries': 'console_libraries',
    'web.console.templates': 'consoles',
    'web.cors.origin': '.*',
    'web.enable-admin-api': 'false',
    'web.enable-lifecycle': 'false',
    'web.external-url': '',
    'web.listen-address': '0.0.0.0:9090',
    'web.max-connections': '512',
    'web.page-title': 'Prometheus Time Series Collection and Processing Server',
    'web.read-timeout': '5m',
    'web.route-prefix': '/',
    'web.user-assets': '',
  },
};

describe('Flags', () => {
  beforeEach(() => {
    fetch.resetMocks();
  });

  describe('before data is returned', () => {
    it('renders a spinner', () => {
      const flags = shallow(<Flags />);
      const icon = flags.find(FontAwesomeIcon);
      expect(icon.prop('icon')).toEqual(faSpinner);
      expect(icon.prop('spin')).toEqual(true);
    });
  });

  describe('when data is returned', () => {
    it('renders a table', async () => {
      const mock = fetch.mockResponse(JSON.stringify(sampleFlagsResponse));

      let flags: ReactWrapper;
      await act(async () => {
        flags = mount(<Flags />);
      });
      flags.update();

      expect(mock).toHaveBeenCalledWith('../api/v1/status/flags');
      const table = flags.find(Table);
      expect(table.prop('striped')).toBe(true);

      const rows = flags.find('tr');
      const keys = Object.keys(sampleFlagsResponse.data);
      expect(rows.length).toBe(keys.length);
      for (let i = 0; i < keys.length; i++) {
        const row = rows.at(i);
        expect(row.find('th').text()).toBe(keys[i]);
        expect(row.find('td').text()).toBe(sampleFlagsResponse.data[keys[i]]);
      }
    });
  });

  describe('when an error is returned', () => {
    it('displays an alert', async () => {
      const mock = fetch.mockReject(new Error('error loading flags'));

      let flags: ReactWrapper;
      await act(async () => {
        flags = mount(<Flags />);
      });
      flags.update();

      expect(mock).toHaveBeenCalledWith('../api/v1/status/flags');
      const alert = flags.find(Alert);
      expect(alert.prop('color')).toBe('danger');
      expect(alert.text()).toContain('error loading flags');
    });
  });
});
