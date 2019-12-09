import * as React from 'react';
import { mount, shallow } from 'enzyme';
import Panel, { PanelOptions, PanelType } from './Panel';
import ExpressionInput from './ExpressionInput';
import GraphControls from './graph/GraphControls';
import { NavLink, TabPane } from 'reactstrap';
import TimeInput from './TimeInput';
import DataTable from './DataTable';
import { GraphTabContent } from './graph/GraphTabContent';

const defaultProps = {
  options: {
    expr: 'prometheus_engine',
    type: PanelType.Table,
    range: 10,
    endTime: 1572100217898,
    resolution: 28,
    stacked: false,
  },
  onOptionsChanged: (): void => {},
  pastQueries: [],
  metricNames: [
    'prometheus_engine_queries',
    'prometheus_engine_queries_concurrent_max',
    'prometheus_engine_query_duration_seconds',
  ],
  removePanel: (): void => {},
  onExecuteQuery: (): void => {},
};

describe('Panel', () => {
  const panel = shallow(<Panel {...defaultProps} />);

  it('renders an ExpressionInput', () => {
    const input = panel.find(ExpressionInput);
    expect(input.prop('value')).toEqual('prometheus_engine');
    expect(input.prop('autocompleteSections')).toEqual({
      'Metric Names': [
        'prometheus_engine_queries',
        'prometheus_engine_queries_concurrent_max',
        'prometheus_engine_query_duration_seconds',
      ],
      'Query History': [],
    });
  });

  it('renders NavLinks', () => {
    const results: PanelOptions[] = [];
    const onOptionsChanged = (opts: PanelOptions): void => {
      results.push(opts);
    };
    const panel = shallow(<Panel {...defaultProps} onOptionsChanged={onOptionsChanged} />);
    const links = panel.find(NavLink);
    [{ panelType: 'Table', active: true }, { panelType: 'Graph', active: false }].forEach(
      (tc: { panelType: string; active: boolean }, i: number) => {
        const link = links.at(i);
        const className = tc.active ? 'active' : '';
        expect(link.prop('className')).toEqual(className);
        link.simulate('click');
        expect(results).toHaveLength(1);
        expect(results[0].type).toEqual(tc.panelType.toLowerCase());
        results.pop();
      }
    );
  });

  it('renders a TabPane with a TimeInput and a DataTable when in table mode', () => {
    const tab = panel.find(TabPane).filterWhere(tab => tab.prop('tabId') === 'table');
    const timeInput = tab.find(TimeInput);
    expect(timeInput.prop('time')).toEqual(defaultProps.options.endTime);
    expect(timeInput.prop('range')).toEqual(defaultProps.options.range);
    expect(timeInput.prop('placeholder')).toEqual('Evaluation time');
    expect(tab.find(DataTable)).toHaveLength(1);
  });

  it('renders a TabPane with a Graph and GraphControls when in graph mode', () => {
    const options = {
      expr: 'prometheus_engine',
      type: PanelType.Graph,
      range: 10,
      endTime: 1572100217898,
      resolution: 28,
      stacked: false,
    };
    const graphPanel = mount(<Panel {...defaultProps} options={options} />);
    const controls = graphPanel.find(GraphControls);
    graphPanel.setState({ data: { resultType: 'matrix', result: [] } });
    const graph = graphPanel.find(GraphTabContent);
    expect(controls.prop('endTime')).toEqual(options.endTime);
    expect(controls.prop('range')).toEqual(options.range);
    expect(controls.prop('resolution')).toEqual(options.resolution);
    expect(controls.prop('stacked')).toEqual(options.stacked);
    expect(graph.prop('stacked')).toEqual(options.stacked);
  });

  describe('when switching between modes', () => {
    const panel = shallow(<Panel {...defaultProps} />);
    it('nulls out data', () => {
      panel.setState({ data: 'somedata' });
      expect(panel.state('data')).toEqual('somedata');
      panel.setProps({ options: { ...defaultProps.options, type: PanelType.Graph } });
      expect(panel.state('data')).toBeNull();
    });
  });

  describe('callbacks', () => {
    const panel = shallow(<Panel {...defaultProps} />);
    const instance: any = panel.instance();
    const executeQuerySpy = jest.spyOn(instance, 'executeQuery');
    [
      instance.handleChangeRange,
      instance.handleChangeEndTime,
      instance.handleChangeResolution,
      instance.handleChangeStacking,
    ].forEach(callback => {
      it(`executes query during ${callback}`, () => {
        callback();
        expect(executeQuerySpy).toHaveBeenCalledTimes(1);
        executeQuerySpy.mockReset();
      });
    });

    [instance.handleExpressionChange, instance.handleChangeType].forEach(callback => {
      it(`does NOT execute query during ${callback}`, () => {
        callback();
        expect(executeQuerySpy).toHaveBeenCalledTimes(0);
        executeQuerySpy.mockReset();
      });
    });
  });

  describe('when changing query then time', () => {
    it('executes the new query', () => {
      const initialExpr = 'time()';
      const newExpr = 'time() - time()';
      const panel = shallow(<Panel {...defaultProps} options={{ ...defaultProps.options, expr: initialExpr }} />);
      const instance: any = panel.instance();
      instance.executeQuery();
      const executeQuerySpy = jest.spyOn(instance, 'executeQuery');
      instance.handleExpressionChange(newExpr);
      expect(executeQuerySpy).toHaveBeenCalledTimes(0); //change query without executing
      instance.handleChangeEndTime(1575744840);
      expect(executeQuerySpy).toHaveBeenCalledTimes(1); //execute query implicitly with time change
    });
  });
});
