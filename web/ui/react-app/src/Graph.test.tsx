import * as React from 'react';
import { shallow } from 'enzyme';
import Graph from './Graph';
import { Alert } from 'reactstrap';
import ReactResizeDetector from 'react-resize-detector';

describe('Graph', () => {
  it('renders an alert if data result type is different than "matrix"', () => {
    const props: any = {
      data: { resultType: 'invalid', result: [] },
      stacked: false,
      queryParams: {
        startTime: 1572100210000,
        endTime: 1572100217898,
        resolution: 10,
      },
      color: 'danger',
      children: `Query result is of wrong type '`,
    };
    const graph = shallow(<Graph {...props} />);
    const alert = graph.find(Alert);
    expect(alert.prop('color')).toEqual(props.color);
    expect(alert.childAt(0).text()).toEqual(props.children);
  });

  it('renders an alert if data result empty', () => {
    const props: any = {
      data: {
        resultType: 'matrix',
        result: [],
      },
      color: 'secondary',
      children: 'Empty query result',
      stacked: false,
      queryParams: {
        startTime: 1572100210000,
        endTime: 1572100217898,
        resolution: 10,
      },
    };
    const graph = shallow(<Graph {...props} />);
    const alert = graph.find(Alert);
    expect(alert.prop('color')).toEqual(props.color);
    expect(alert.childAt(0).text()).toEqual(props.children);
  });

  describe('data is returned', () => {
    const props: any = {
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
      expect(resize.prop('handleWidth')).toBe(true);
      expect(div).toHaveLength(1);
      expect(innerdiv).toHaveLength(1);
    });
    it('formats tick values correctly', () => {
      const graph = new Graph({ data: { result: [] }, queryParams: {} } as any);
      [
        { input: 2e24, output: '2.00Y' },
        { input: 2e23, output: '200.00Z' },
        { input: 2e22, output: '20.00Z' },
        { input: 2e21, output: '2.00Z' },
        { input: 2e19, output: '20.00E' },
        { input: 2e18, output: '2.00E' },
        { input: 2e17, output: '200.00P' },
        { input: 2e16, output: '20.00P' },
        { input: 2e15, output: '2.00P' },
        { input: 1e15, output: '1.00P' },
        { input: 2e14, output: '200.00T' },
        { input: 2e13, output: '20.00T' },
        { input: 2e12, output: '2.00T' },
        { input: 2e11, output: '200.00G' },
        { input: 2e10, output: '20.00G' },
        { input: 2e9, output: '2.00G' },
        { input: 2e8, output: '200.00M' },
        { input: 2e7, output: '20.00M' },
        { input: 2e6, output: '2.00M' },
        { input: 2e5, output: '200.00k' },
        { input: 2e4, output: '20.00k' },
        { input: 2e3, output: '2.00k' },
        { input: 2e2, output: '200.00' },
        { input: 2e1, output: '20.00' },
        { input: 2, output: '2.00' },
        { input: 2e-1, output: '0.20' },
        { input: 2e-2, output: '0.02' },
        { input: 2e-3, output: '2.00m' },
        { input: 2e-4, output: '0.20m' },
        { input: 2e-5, output: '0.02m' },
        { input: 2e-6, output: '2.00µ' },
        { input: 2e-7, output: '0.20µ' },
        { input: 2e-8, output: '0.02µ' },
        { input: 2e-9, output: '2.00n' },
        { input: 2e-10, output: '0.20n' },
        { input: 2e-11, output: '0.02n' },
        { input: 2e-12, output: '2.00p' },
        { input: 2e-13, output: '0.20p' },
        { input: 2e-14, output: '0.02p' },
        { input: 2e-15, output: '2.00f' },
        { input: 2e-16, output: '0.20f' },
        { input: 2e-17, output: '0.02f' },
        { input: 2e-18, output: '2.00a' },
        { input: 2e-19, output: '0.20a' },
        { input: 2e-20, output: '0.02a' },
        { input: 1e-21, output: '1.00z' },
        { input: 2e-21, output: '2.00z' },
        { input: 2e-22, output: '0.20z' },
        { input: 2e-23, output: '0.02z' },
        { input: 2e-24, output: '2.00y' },
        { input: 2e-25, output: '0.20y' },
        { input: 2e-26, output: '0.02y' },
      ].map(t => {
        expect(graph.formatValue(t.input)).toBe(t.output);
      });
    });
    describe('Legend', () => {
      it('renders a legend', () => {
        const graph = shallow(<Graph {...props} />);
        expect(graph.find('.graph-legend .legend-item')).toHaveLength(1);
      });
    });
  });
});
