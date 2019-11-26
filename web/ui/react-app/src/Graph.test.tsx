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
  describe('formatValue', () => {
    it('formats tick values correctly', () => {
      const graph = new Graph({ data: { result: [] }, queryParams: {} } as any);
      [
        { input: null, output: 'null' },
        { input: 0, output: '0.00' },
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
    it('should throw error if no match', () => {
      const graph = new Graph({ data: { result: [] }, queryParams: {} } as any);
      try {
        graph.formatValue(undefined as any);
      } catch (error) {
        expect(error.message).toEqual("couldn't format a value, this is a bug");
      }
    });
  });
  describe('getColors', () => {
    it('should generate proper colors', () => {
      const graph = new Graph({ data: { result: [{}, {}, {}, {}, {}, {}, {}, {}, {}] }, queryParams: {} } as any);
      expect(
        graph
          .getColors()
          .map(c => c.toString())
          .join(',')
      ).toEqual(
        'rgb(237,194,64),rgb(175,216,248),rgb(203,75,75),rgb(77,167,77),rgb(148,64,237),rgb(189,155,51),rgb(140,172,198),rgb(162,60,60),rgb(61,133,61)'
      );
    });
  });
  describe('on component update', () => {
    let graph: any;
    beforeEach(() => {
      jest.spyOn($, 'plot').mockImplementation(() => {});
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
  describe('parseValue', () => {
    it('should parse number properly', () => {
      expect(new Graph({ stacked: true, data: { result: [] }, queryParams: {} } as any).parseValue('12.3e')).toEqual(12.3);
    });
    it('should return 0 if value is NaN and stacked prop is true', () => {
      expect(new Graph({ stacked: true, data: { result: [] }, queryParams: {} } as any).parseValue('asd')).toEqual(0);
    });
    it('should return null if value is NaN and stacked prop is false', () => {
      expect(new Graph({ stacked: false, data: { result: [] }, queryParams: {} } as any).parseValue('asd')).toBeNull();
    });
  });

  describe('plot', () => {
    let spyFlot: any;
    beforeEach(() => {
      spyFlot = jest.spyOn($, 'plot').mockImplementation(() => {});
    });
    afterAll(() => {
      jest.restoreAllMocks();
    });
    it('should not call jquery.plot if charRef not exist', () => {
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
    it('should call jquery.plot if charRef exist', () => {
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
  describe('Plot options', () => {
    it('should configer options properly if stacked prop is true', () => {
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
      expect((wrapper.instance() as any).getOptions()).toMatchObject({
        series: {
          stack: true,
          lines: { lineWidth: 1, steps: false, fill: true },
          shadowSize: 0,
        },
      });
    });
    it('should configer options properly if stacked prop is false', () => {
      const wrapper = shallow(
        <Graph
          {...({
            stacked: false,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572130692,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      expect((wrapper.instance() as any).getOptions()).toMatchObject({
        series: {
          stack: false,
          lines: { lineWidth: 2, steps: false, fill: false },
          shadowSize: 0,
        },
      });
    });
    it('should return proper tooltip html from options', () => {
      const wrapper = shallow(
        <Graph
          {...({
            stacked: false,
            queryParams: {
              startTime: 1572128592,
              endTime: 1572130692,
              resolution: 28,
            },
            data: { result: [{ values: [], metric: {} }] },
          } as any)}
        />
      );
      expect(
        (wrapper.instance() as any)
          .getOptions()
          .tooltip.content('', 1572128592, 1572128592, { series: { labels: { foo: 1, bar: 2 }, color: '' } })
      ).toEqual(`
            <div class="date">Mon, 19 Jan 1970 04:42:08 GMT</div>
            <div>
              <span class="detail-swatch" style="background-color: " />
              <span>value: <strong>1572128592</strong></span>
            <div>
            <div class="labels mt-1">
              <div class="mb-1"><strong>foo</strong>: 1</div><div class="mb-1"><strong>bar</strong>: 2</div>
            </div>
          `);
    });
    it('should render Plot with proper options', () => {
      const wrapper = mount(
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
      expect((wrapper.instance() as any).getOptions()).toEqual({
        grid: {
          hoverable: true,
          clickable: true,
          autoHighlight: true,
          mouseActiveRadius: 100,
        },
        legend: { show: false },
        xaxis: {
          mode: 'time',
          showTicks: true,
          showMinorTicks: true,
          timeBase: 'milliseconds',
        },
        yaxis: { tickFormatter: expect.anything() },
        crosshair: { mode: 'xy', color: '#bbb' },
        tooltip: {
          show: true,
          cssClass: 'graph-tooltip',
          content: expect.anything(),
          defaultTheme: false,
          lines: true,
        },
        series: {
          stack: true,
          lines: { lineWidth: 1, steps: false, fill: true },
          shadowSize: 0,
        },
      });
    });
  });
});
