import * as React from 'react';
import { shallow } from 'enzyme';
import { Status } from '../pages';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import * as useFetch from '../pages/useFetch';
import renderer from 'react-test-renderer';

describe('Status', () => {
  afterEach(() => jest.restoreAllMocks());

  it('should render spinner while waiting data', () => {
    const wrapper = shallow(<Status />);
    expect(wrapper.find(FontAwesomeIcon)).toHaveLength(1);
  });
  it('should render Alert on error', () => {
    jest.spyOn(useFetch, 'default').mockImplementation(() => ({ error: new Error('foo') } as any));
    const wrapper = shallow(<Status />);
    expect(wrapper.find(Alert)).toHaveLength(1);
  });
  it('should fetch proper API endpoints', () => {
    const useFetchSpy = jest.spyOn(useFetch, 'default');
    shallow(<Status />);
    expect(useFetchSpy).toHaveBeenCalledWith(['../api/v1/runtimeinfo', '../api/v1/buildinfo', '../api/v1/alertmanagers']);
  });
  describe('Status Table', () => {
    const response = [
      {
        StartTime: '2019-10-30T22:03:23.247913868+02:00',
        CWD: '/home/boyskila/Desktop/prometheus',
        GoroutineCount: 37,
        GOMAXPROCS: 4,
        GOGC: '',
        GODEBUG: '',
        CorruptionCount: 0,
        ChunkCount: 1383,
        TimeSeriesCount: 461,
        LastConfigTime: '2019-10-30T22:03:23+02:00',
        ReloadConfigSuccess: true,
        StorageRetention: '15d',
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

    afterEach(() => jest.restoreAllMocks());

    it('should display 3 tables', () => {
      jest.spyOn(useFetch, 'default').mockImplementation(() => ({ response } as any));
      const wrapper = shallow(<Status />);
      expect(wrapper.find(Table)).toHaveLength(3);
    });
    it('should titles have proper positions', () => {
      jest.spyOn(useFetch, 'default').mockImplementation(() => ({ response } as any));
      const wrapper = shallow(<Status />);
      expect(
        wrapper
          .find('h2')
          .at(0)
          .text()
      ).toEqual('Runtime Information');
      expect(
        wrapper
          .find('h2')
          .at(1)
          .text()
      ).toEqual('Build Information');
      expect(
        wrapper
          .find('h2')
          .at(2)
          .text()
      ).toEqual('Alertmanagers');
    });
    it('should match table snapshot', () => {
      jest.spyOn(useFetch, 'default').mockImplementation(() => ({ response } as any));
      const wrapper = renderer.create(<Status />);
      expect(wrapper.toJSON()).toMatchSnapshot();
    });
  });
});
