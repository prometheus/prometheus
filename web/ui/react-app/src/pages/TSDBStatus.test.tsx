import * as React from 'react';
import { mount, shallow, ReactWrapper } from 'enzyme';
import { act } from 'react-dom/test-utils';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { TSDBStatus } from '.';
import { TSDBMap } from './TSDBStatus';

const fakeTSDBStatusResponse: {
  status: string;
  data: TSDBMap;
} = {
  status: 'success',
  data: {
    labelValueCountByLabelName: [
      {
        name: '__name__',
        value: 5,
      },
    ],
    seriesCountByMetricName: [
      {
        name: 'scrape_duration_seconds',
        value: 1,
      },
      {
        name: 'scrape_samples_scraped',
        value: 1,
      },
    ],
    memoryInBytesByLabelName: [
      {
        name: '__name__',
        value: 103,
      },
    ],
    seriesCountByLabelValuePair: [
      {
        name: 'instance=localhost:9100',
        value: 5,
      },
    ],
  },
};

describe('TSDB Stats', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
  });

  it('before data is returned', () => {
    const tsdbStatus = shallow(<TSDBStatus />);
    const icon = tsdbStatus.find(FontAwesomeIcon);
    expect(icon.prop('icon')).toEqual(faSpinner);
    expect(icon.prop('spin')).toBeTruthy();
  });

  describe('when an error is returned', () => {
    it('displays an alert', async () => {
      const mock = fetchMock.mockReject(new Error('error loading tsdb status'));

      let page: any;
      await act(async () => {
        page = mount(<TSDBStatus pathPrefix="/path/prefix" />);
      });
      page.update();

      expect(mock).toHaveBeenCalledWith('/path/prefix/api/v1/status/tsdb', undefined);
      const alert = page.find(Alert);
      expect(alert.prop('color')).toBe('danger');
      expect(alert.text()).toContain('error loading tsdb status');
    });
  });

  describe('Table Data Validation', () => {
    it('Table Test', async () => {
      const tables = [
        {
          data: fakeTSDBStatusResponse.data.labelValueCountByLabelName,
          table_index: 0,
        },
        {
          data: fakeTSDBStatusResponse.data.seriesCountByMetricName,
          table_index: 1,
        },
        {
          data: fakeTSDBStatusResponse.data.memoryInBytesByLabelName,
          table_index: 2,
        },
        {
          data: fakeTSDBStatusResponse.data.seriesCountByLabelValuePair,
          table_index: 3,
        },
      ];

      const mock = fetchMock.mockResponse(JSON.stringify(fakeTSDBStatusResponse));
      let page: any;
      await act(async () => {
        page = mount(<TSDBStatus pathPrefix="/path/prefix" />);
      });
      page.update();

      expect(mock).toHaveBeenCalledWith('/path/prefix/api/v1/status/tsdb', undefined);

      for (let i = 0; i < tables.length; i++) {
        const data = tables[i].data;
        const table = page
          .find(Table)
          .at(tables[i].table_index)
          .find('tbody');
        const rows = table.find('tr');
        for (let i = 0; i < data.length; i++) {
          const firstRowColumns = rows
            .at(i)
            .find('td')
            .map((column: ReactWrapper) => column.text());
          expect(rows.length).toBe(data.length);
          expect(firstRowColumns[0]).toBe(data[i].name);
          expect(firstRowColumns[1]).toBe(data[i].value.toString());
        }
      }
    });
  });
});
