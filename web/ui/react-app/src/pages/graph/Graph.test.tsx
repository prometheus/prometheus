import * as React from 'react';
import $ from 'jquery';
import { shallow, mount } from 'enzyme';
import Graph from './Graph';
import ReactResizeDetector from 'react-resize-detector';
import { Legend } from './Legend';

describe('Graph', () => {
  beforeAll(() => {
    jest.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: any) => cb());
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
    describe('Legend', () => {
      it('renders a legend', () => {
        const graph = shallow(<Graph {...props} />);
        expect(graph.find(Legend)).toHaveLength(1);
      });
    });
  });
  describe('on component update', () => {
    let graph: any;
    let spyState: any;
    let mockPlot: any;
    beforeEach(() => {
      mockPlot = jest.spyOn($, 'plot').mockReturnValue({ setData: jest.fn(), draw: jest.fn(), destroy: jest.fn() } as any);
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
      spyState = jest.spyOn(graph.instance(), 'setState');
    });
    afterEach(() => {
      spyState.mockReset();
      mockPlot.mockReset();
    });
    it('should trigger state update when new data is received', () => {
      graph.setProps({ data: { result: [{ values: [{}], metric: {} }] } });
      expect(spyState).toHaveBeenCalledWith(
        {
          chartData: [
            {
              color: 'rgb(237,194,64)',
              data: [[1572128592000, null]],
              index: 0,
              labels: {},
            },
          ],
        },
        expect.anything()
      );
    });
    it('should trigger state update when stacked prop is changed', () => {
      graph.setProps({ stacked: false });
      expect(spyState).toHaveBeenCalledWith(
        {
          chartData: [
            {
              color: 'rgb(237,194,64)',
              data: [[1572128592000, null]],
              index: 0,
              labels: {},
            },
          ],
        },
        expect.anything()
      );
    });
  });
  describe('on unmount', () => {
    it('should call destroy plot', () => {
      const graph = mount(
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
      const spyPlotDestroy = jest.spyOn(graph.instance(), 'componentWillUnmount');
      graph.unmount();
      expect(spyPlotDestroy).toHaveBeenCalledTimes(1);
      spyPlotDestroy.mockReset();
    });
  });

  describe('plot', () => {
    it('should not call jquery.plot if chartRef not exist', () => {
      const mockSetData = jest.fn();
      jest.spyOn($, 'plot').mockReturnValue({ setData: mockSetData, draw: jest.fn(), destroy: jest.fn() } as any);
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
      expect(mockSetData).not.toBeCalled();
    });
    it('should call jquery.plot if chartRef exist', () => {
      const mockPlot = jest
        .spyOn($, 'plot')
        .mockReturnValue({ setData: jest.fn(), draw: jest.fn(), destroy: jest.fn() } as any);
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
      expect(mockPlot).toBeCalled();
    });
    it('should destroy plot', () => {
      const mockDestroy = jest.fn();
      jest.spyOn($, 'plot').mockReturnValue({ setData: jest.fn(), draw: jest.fn(), destroy: mockDestroy } as any);
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
      expect(mockDestroy).toHaveBeenCalledTimes(2);
    });
  });
  describe('plotSetAndDraw', () => {
    it('should call spyPlotSetAndDraw on legend hover', () => {
      jest.spyOn($, 'plot').mockReturnValue({ setData: jest.fn(), draw: jest.fn(), destroy: jest.fn() } as any);
      const graph = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: {
              result: [
                { values: [], metric: {} },
                { values: [], metric: {} },
              ],
            },
          } as any)}
        />
      );
      (graph.instance() as any).plot(); // create chart
      const spyPlotSetAndDraw = jest.spyOn(graph.instance() as any, 'plotSetAndDraw');
      graph
        .find('.legend-item')
        .at(0)
        .simulate('mouseover');
      expect(spyPlotSetAndDraw).toHaveBeenCalledTimes(1);
    });
    it('should call spyPlotSetAndDraw with chartDate from state as default value', () => {
      const mockSetData = jest.fn();
      const spyPlot = jest
        .spyOn($, 'plot')
        .mockReturnValue({ setData: mockSetData, draw: jest.fn(), destroy: jest.fn() } as any);
      const graph: any = mount(
        <Graph
          {...({
            stacked: true,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572128598,
              resolution: 28,
            },
            data: {
              result: [
                { values: [], metric: {} },
                { values: [], metric: {} },
              ],
            },
          } as any)}
        />
      );
      (graph.instance() as any).plot(); // create chart
      graph.find('.graph-legend').simulate('mouseout');
      expect(mockSetData).toHaveBeenCalledWith(graph.state().chartData);
      spyPlot.mockReset();
    });
  });
});
