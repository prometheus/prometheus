import * as React from 'react';
import $ from 'jquery';
import { shallow, mount } from 'enzyme';
import Graph from './Graph';
import ReactResizeDetector from 'react-resize-detector';

describe('Graph', () => {
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
    describe('Legend', () => {
      it('renders a legend', () => {
        const graph = shallow(<Graph {...props} />);
        expect(graph.find('.graph-legend .legend-item')).toHaveLength(1);
      });
    });
  });
  describe('on component update', () => {
    let graph: any;
    beforeEach(() => {
      jest.spyOn($, 'plot').mockImplementation(() => ({} as any));
      graph = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
    });
    afterAll(() => {
      jest.restoreAllMocks();
    });
    it('should trigger state update when new data is recieved', () => {
      const spyState = jest.spyOn(Graph.prototype, 'setState');
      graph.setProps({ data: { result: [{ values: [{}], metric: {} }] } });
      expect(spyState).toHaveBeenCalledWith(
        {
          chartData: [
            {
              color: 'rgb(237,194,64)',
              data: [[1572128592000, 0]],
              index: 0,
              labels: {},
            },
          ],
          selectedSeriesIndex: null,
        },
        expect.anything()
      );
    });
    it('should trigger state update when stacked prop is changed', () => {
      const spyState = jest.spyOn(Graph.prototype, 'setState');
      graph.setProps({ stacked: true });
      expect(spyState).toHaveBeenCalledWith(
        {
          chartData: [
            {
              color: 'rgb(237,194,64)',
              data: [[1572128592000, 0]],
              index: 0,
              labels: {},
            },
          ],
          selectedSeriesIndex: null,
        },
        expect.anything()
      );
    });
  });
  describe('on unmount', () => {
    it('should call destroy plot', () => {
      const wrapper = shallow(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572130692,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      const spyPlotDestroy = jest.spyOn(Graph.prototype, 'componentWillUnmount');
      wrapper.unmount();
      expect(spyPlotDestroy).toHaveBeenCalledTimes(1);
    });
  });

  describe('plot', () => {
    let spyFlot: any;
    beforeEach(() => {
      spyFlot = jest.spyOn($, 'plot').mockImplementation(() => ({} as any));
    });
    afterAll(() => {
      jest.restoreAllMocks();
    });
    it('should not call jquery.plot if chartRef not exist', () => {
      const graph = shallow(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      (graph.instance() as any).plot();
      expect(spyFlot).not.toBeCalled();
    });
    it('should call jquery.plot if chartRef exist', () => {
      const graph = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      (graph.instance() as any).plot();
      expect(spyFlot).toBeCalled();
    });
    it('should destroy plot', () => {
      const spyPlotDestroy = jest.fn();
      jest.spyOn($, 'plot').mockReturnValue({ destroy: spyPlotDestroy } as any);
      const graph = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      (graph.instance() as any).plot();
      (graph.instance() as any).destroyPlot();
      expect(spyPlotDestroy).toHaveBeenCalledTimes(1);
      jest.restoreAllMocks();
    });
  });
  describe('plotSetAndDraw', () => {
    afterEach(() => {
      jest.restoreAllMocks();
    });
    it('should call spyPlotSetAndDraw on legend hover', () => {
      jest.spyOn($, 'plot').mockReturnValue({ setData: jest.fn(), draw: jest.fn() } as any);
      jest.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: any) => cb());
      const graph = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }, { values: [], metric: {} }] },
          } as any)}
        />
      );
      (graph.instance() as any).plot(); // create chart
      const spyPlotSetAndDraw = jest.spyOn(Graph.prototype, 'plotSetAndDraw');
      graph
        .find('.legend-item')
        .at(0)
        .simulate('mouseover');
      expect(spyPlotSetAndDraw).toHaveBeenCalledTimes(1);
    });
    it('should call spyPlotSetAndDraw with chartDate from state as default value', () => {
      const spySetData = jest.fn();
      jest.spyOn($, 'plot').mockReturnValue({ setData: spySetData, draw: jest.fn() } as any);
      jest.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: any) => cb());
      const graph: any = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }, { values: [], metric: {} }] },
          } as any)}
        />
      );
      (graph.instance() as any).plot(); // create chart
      graph.find('.graph-legend').simulate('mouseout');
      expect(spySetData).toHaveBeenCalledWith(graph.state().chartData);
    });
  });
});
