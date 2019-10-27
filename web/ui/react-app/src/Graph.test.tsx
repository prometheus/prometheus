import * as React from 'react';
import { shallow } from 'enzyme';
import Graph from './Graph';
import { Alert } from 'reactstrap';
import ReactResizeDetector from 'react-resize-detector';
import Legend from './Legend';

describe('Graph', () => {
  [
    {
      data: null,
      color: 'light',
      children: 'No data queried yet',
    },
    {
      data: { resultType: 'invalid' },
      color: 'danger',
      children: `Query result is of wrong type '`,
    },
    {
      data: {
        resultType: 'matrix',
        result: [],
      },
      color: 'secondary',
      children: 'Empty query result',
    },
  ].forEach(testCase => {
    it(`renders an alert if data is "${testCase.data}"`, () => {
      const props = {
        data: testCase.data,
        stacked: false,
        queryParams: {
          startTime: 1572100210000,
          endTime: 1572100217898,
          resolution: 10,
        },
      };
      const graph = shallow(<Graph {...props} />);
      const alert = graph.find(Alert);
      expect(alert.prop('color')).toEqual(testCase.color);
      expect(alert.childAt(0).text()).toEqual(testCase.children);
    });
  });

  describe('data is returned', () => {
    const props = {
      queryParams: {
        startTime: 1572128592,
        endTime: 1572130692,
        resolution: 28,
      },
      stacked: false,
      data: {
        resultType: 'matrix',
        result: [
          {
            metric: {
              code: '200',
              handler: '/graph',
              instance: 'localhost:9090',
              job: 'prometheus',
            },
            values: [
              [1572128592, '23'],
              [1572128620, '2'],
              [1572128648, '4'],
              [1572128676, '1'],
              [1572128704, '2'],
              [1572128732, '12'],
              [1572128760, '1'],
              [1572128788, '0'],
              [1572128816, '0'],
              [1572128844, '2'],
              [1572128872, '5'],
              [1572130384, '6'],
              [1572130412, '7'],
              [1572130440, '19'],
              [1572130468, '33'],
              [1572130496, '14'],
              [1572130524, '7'],
              [1572130552, '6'],
              [1572130580, '0'],
              [1572130608, '0'],
              [1572130636, '0'],
              [1572130664, '0'],
              [1572130692, '0'],
            ],
          },
        ],
      },
    };
    it('renders a graph with props', () => {
      const graph = shallow(<Graph {...props} />);
      const div = graph.find('div').filterWhere(elem => elem.prop('className') === 'graph');
      const resize = div.find(ReactResizeDetector);
      const innerdiv = div.find('div').filterWhere(elem => elem.prop('className') === 'graph-chart');
      const legend = graph.find(Legend);
      expect(resize.prop('handleWidth')).toBe(true);
      expect(div).toHaveLength(1);
      expect(innerdiv).toHaveLength(1);
      expect(legend).toHaveLength(1);
    });
  });
});
