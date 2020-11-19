import * as React from 'react';
import { mount, ReactWrapper } from 'enzyme';
import { act } from 'react-dom/test-utils';
import { Table } from 'reactstrap';

import TSDBStatus from './TSDBStatus';
import { TSDBMap } from './TSDBStatus';
import { PathPrefixContext } from '../../contexts/PathPrefixContext';

const fakeTSDBStatusResponse: {
  status: string;
  data: TSDBMap;
} = {
  status: 'success',
  data: {
    headStats: {
      numSeries: 508,
      chunkCount: 937,
      minTime: 1591516800000,
      maxTime: 1598896800143,
    },
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

  describe('Table Data Validation', () => {
    it('Table Test', async () => {
      const tables = [
        fakeTSDBStatusResponse.data.labelValueCountByLabelName,
        fakeTSDBStatusResponse.data.seriesCountByMetricName,
        fakeTSDBStatusResponse.data.memoryInBytesByLabelName,
        fakeTSDBStatusResponse.data.seriesCountByLabelValuePair,
      ];

      const mock = fetchMock.mockResponse(JSON.stringify(fakeTSDBStatusResponse));
      let page: any;
      await act(async () => {
        page = mount(
          <PathPrefixContext.Provider value="/path/prefix">
            <TSDBStatus />
          </PathPrefixContext.Provider>
        );
      });
      page.update();

      expect(mock).toHaveBeenCalledWith('/path/prefix/api/v1/status/tsdb', {
        cache: 'no-store',
        credentials: 'same-origin',
      });

      const headStats = page
        .find(Table)
        .at(0)
        .find('tbody')
        .find('td');
      ['508', '937', '2020-06-07T08:00:00.000Z (1591516800000)', '2020-08-31T18:00:00.143Z (1598896800143)'].forEach(
        (value, i) => {
          expect(headStats.at(i).text()).toEqual(value);
        }
      );

      for (let i = 0; i < tables.length; i++) {
        const data = tables[i];
        const table = page
          .find(Table)
          .at(i + 1)
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
