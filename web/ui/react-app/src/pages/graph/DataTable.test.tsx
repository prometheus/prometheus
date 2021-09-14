import * as React from 'react';
import { shallow } from 'enzyme';
import DataTable, { QueryResult } from './DataTable';
import { Alert, Table } from 'reactstrap';
import SeriesName from './SeriesName';

describe('DataTable', () => {
  describe('when data is null', () => {
    it('renders an alert', () => {
      const table = shallow(<DataTable data={null} />);
      const alert = table.find(Alert);
      expect(Object.keys(alert.props())).toHaveLength(7);
      expect(alert.prop('color')).toEqual('light');
      expect(alert.prop('children')).toEqual('No data queried yet');
    });
  });

  describe('when data.result is empty', () => {
    it('renders an alert', () => {
      const dataTableProps: QueryResult = {
        data: {
          resultType: 'vector',
          result: [],
        },
      };
      const table = shallow(<DataTable {...dataTableProps} />);
      const alert = table.find(Alert);
      expect(Object.keys(alert.props())).toHaveLength(7);
      expect(alert.prop('color')).toEqual('secondary');
      expect(alert.prop('children')).toEqual('Empty query result');
    });
  });

  describe('when resultType is a vector with values', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'vector',
        result: [
          {
            metric: {
              __name__: 'metric_name_1',
              label1: 'value_1',
              labeln: 'value_n',
            },
            value: [1572098246.599, '0'],
          },
          {
            metric: {
              __name__: 'metric_name_2',
              label1: 'value_1',
              labeln: 'value_n',
            },
            value: [1572098246.599, '1'],
          },
        ],
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);

    it('renders a table', () => {
      const table = dataTable.find(Table);
      expect(table.prop('hover')).toBe(true);
      expect(table.prop('size')).toEqual('sm');
      expect(table.prop('className')).toEqual('data-table');
      expect(table.find('tbody')).toHaveLength(1);
    });

    it('renders rows', () => {
      const table = dataTable.find(Table);
      table.find('tr').forEach((row, idx) => {
        expect(row.find(SeriesName)).toHaveLength(1);
        expect(row.find('td').at(1).text()).toEqual(`${idx}`);
      });
    });
  });

  describe('when resultType is a vector with too many values', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'vector',
        result: Array.from(Array(10001).keys()).map((i) => {
          return {
            metric: {
              __name__: `metric_name_${i}`,
              label1: 'value_1',
              labeln: 'value_n',
            },
            value: [1572098246.599, `${i}`],
          };
        }),
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);

    it('renders limited rows', () => {
      const table = dataTable.find(Table);
      expect(table.find('tr')).toHaveLength(10000);
    });

    it('renders a warning', () => {
      const alerts = dataTable.find(Alert);
      expect(alerts.first().render().text()).toEqual('Warning: Fetched 10001 metrics, only displaying first 10000.');
    });
  });

  describe('when resultType is vector and size is more than maximum limit of formatting', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'vector',
        result: Array.from(Array(1001).keys()).map((i) => {
          return {
            metric: {
              __name__: `metric_name_${i}`,
              label1: 'value_1',
              labeln: 'value_n',
            },
            value: [1572098246.599, `${i}`],
          };
        }),
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);

    it('renders a warning', () => {
      const alerts = dataTable.find(Alert);
      expect(alerts.first().render().text()).toEqual(
        'Notice: Showing more than 1000 series, turning off label formatting for performance reasons.'
      );
    });
  });

  describe('when result type is a matrix', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'matrix',
        result: [
          {
            metric: {
              __name__: 'promhttp_metric_handler_requests_total',
              code: '200',
              instance: 'localhost:9090',
              job: 'prometheus',
            },
            values: [
              [1572097950.93, '9'],
              [1572097965.931, '10'],
              [1572097980.929, '11'],
              [1572097995.931, '12'],
              [1572098010.932, '13'],
              [1572098025.933, '14'],
              [1572098040.93, '15'],
              [1572098055.93, '16'],
              [1572098070.93, '17'],
              [1572098085.936, '18'],
              [1572098100.936, '19'],
              [1572098115.933, '20'],
              [1572098130.932, '21'],
              [1572098145.932, '22'],
              [1572098160.933, '23'],
              [1572098175.934, '24'],
              [1572098190.937, '25'],
              [1572098205.934, '26'],
              [1572098220.933, '27'],
              [1572098235.934, '28'],
            ],
          },
          {
            metric: {
              __name__: 'promhttp_metric_handler_requests_total',
              code: '500',
              instance: 'localhost:9090',
              job: 'prometheus',
            },
            values: [
              [1572097950.93, '0'],
              [1572097965.931, '0'],
              [1572097980.929, '0'],
              [1572097995.931, '0'],
              [1572098010.932, '0'],
              [1572098025.933, '0'],
              [1572098040.93, '0'],
              [1572098055.93, '0'],
              [1572098070.93, '0'],
              [1572098085.936, '0'],
              [1572098100.936, '0'],
              [1572098115.933, '0'],
              [1572098130.932, '0'],
              [1572098145.932, '0'],
              [1572098160.933, '0'],
              [1572098175.934, '0'],
              [1572098190.937, '0'],
              [1572098205.934, '0'],
              [1572098220.933, '0'],
              [1572098235.934, '0'],
            ],
          },
          {
            metric: {
              __name__: 'promhttp_metric_handler_requests_total',
              code: '503',
              instance: 'localhost:9090',
              job: 'prometheus',
            },
            values: [
              [1572097950.93, '0'],
              [1572097965.931, '0'],
              [1572097980.929, '0'],
              [1572097995.931, '0'],
              [1572098010.932, '0'],
              [1572098025.933, '0'],
              [1572098040.93, '0'],
              [1572098055.93, '0'],
              [1572098070.93, '0'],
              [1572098085.936, '0'],
              [1572098100.936, '0'],
              [1572098115.933, '0'],
              [1572098130.932, '0'],
              [1572098145.932, '0'],
              [1572098160.933, '0'],
              [1572098175.934, '0'],
              [1572098190.937, '0'],
              [1572098205.934, '0'],
              [1572098220.933, '0'],
              [1572098235.934, '0'],
            ],
          },
        ],
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);
    it('renders rows', () => {
      const table = dataTable.find(Table);
      const rows = table.find('tr');
      expect(table.find('tr')).toHaveLength(3);
      const row = rows.at(0);
      expect(row.text()).toEqual(`<SeriesName />9 @1572097950.93
10 @1572097965.931
11 @1572097980.929
12 @1572097995.931
13 @1572098010.932
14 @1572098025.933
15 @1572098040.93
16 @1572098055.93
17 @1572098070.93
18 @1572098085.936
19 @1572098100.936
20 @1572098115.933
21 @1572098130.932
22 @1572098145.932
23 @1572098160.933
24 @1572098175.934
25 @1572098190.937
26 @1572098205.934
27 @1572098220.933
28 @1572098235.934`);
    });
  });

  describe('when resultType is a scalar', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'scalar',
        result: [1572098246.599, '5'],
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);
    it('renders a scalar row', () => {
      const table = dataTable.find(Table);
      const rows = table.find('tr');
      expect(rows.text()).toEqual('scalar5');
    });
  });

  describe('when resultType is a string', () => {
    const dataTableProps: QueryResult = {
      data: {
        resultType: 'string',
        result: 'string',
      },
    };
    const dataTable = shallow(<DataTable {...dataTableProps} />);
    it('renders a string row', () => {
      const table = dataTable.find(Table);
      const rows = table.find('tr');
      expect(rows.text()).toEqual('stringt');
    });
  });
});
