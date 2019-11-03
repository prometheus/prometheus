import * as React from 'react';
import { shallow } from 'enzyme';
import { Status } from '.';
import { Alert } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import * as useFetch from '../hooks/useFetches';
import toJson from 'enzyme-to-json';

describe('Status', () => {
  afterEach(() => jest.restoreAllMocks());
  it('should render spinner while waiting data', () => {
    const wrapper = shallow(<Status />);
    expect(wrapper.find(FontAwesomeIcon)).toHaveLength(1);
  });
  it('should render Alert on error', () => {
    (useFetch as any).default = jest.fn().mockImplementation(() => ({ error: new Error('foo') }));
    const wrapper = shallow(<Status />);
    expect(wrapper.find(Alert)).toHaveLength(1);
  });
  it('should fetch proper API endpoints', () => {
    const useFetchSpy = jest.spyOn(useFetch, 'default');
    shallow(<Status pathPrefix="/path/prefix" />);
    expect(useFetchSpy).toHaveBeenCalledWith([
      '/path/prefix/api/v1/status/runtimeinfo',
      '/path/prefix/api/v1/status/buildinfo',
      '/path/prefix/api/v1/alertmanagers',
    ]);
  });
  describe('Snapshot testing', () => {
    const response = [
      {
        startTime: '2019-10-30T22:03:23.247913868+02:00',
        CWD: '/home/boyskila/Desktop/prometheus',
        reloadConfigSuccess: true,
        lastConfigTime: '2019-10-30T22:03:23+02:00',
        chunkCount: 1383,
        timeSeriesCount: 461,
        corruptionCount: 0,
        goroutineCount: 37,
        GOMAXPROCS: 4,
        GOGC: '',
        GODEBUG: '',
        storageRetention: '15d',
      },
      {
        version: '',
        revision: '',
        branch: '',
        buildUser: '',
        buildDate: '',
        goVersion: 'go1.13.3',
      },
      {
        activeAlertmanagers: [
          { url: 'https://1.2.3.4:9093/api/v1/alerts' },
          { url: 'https://1.2.3.5:9093/api/v1/alerts' },
          { url: 'https://1.2.3.6:9093/api/v1/alerts' },
          { url: 'https://1.2.3.7:9093/api/v1/alerts' },
          { url: 'https://1.2.3.8:9093/api/v1/alerts' },
          { url: 'https://1.2.3.9:9093/api/v1/alerts' },
        ],
        droppedAlertmanagers: [],
      },
    ];
    it('should match table snapshot', () => {
      (useFetch as any).default = jest.fn().mockImplementation(() => ({ response }));
      const wrapper = shallow(<Status />);
      expect(toJson(wrapper)).toMatchSnapshot();
      jest.restoreAllMocks();
    });
  });
});
