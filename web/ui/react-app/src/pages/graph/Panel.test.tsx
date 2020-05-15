import * as React from 'react';
import { mount, shallow } from 'enzyme';
import Panel, { PanelOptions, PanelType } from './Panel';
import ExpressionInput from './ExpressionInput';
import GraphControls from './GraphControls';
import { NavLink, TabPane } from 'reactstrap';
import TimeInput from './TimeInput';
import DataTable from './DataTable';
import { GraphTabContent } from './GraphTabContent';

const defaultProps = {
  options: {
    expr: 'prometheus_engine',
    type: PanelType.Table,
    range: 10,
    endTime: 1572100217898,
    resolution: 28,
    stacked: false,
  },
  onOptionsChanged: (): void => {
    // Do nothing.
  },
  pastQueries: [],
  metricNames: [
    'prometheus_engine_queries',
    'prometheus_engine_queries_concurrent_max',
    'prometheus_engine_query_duration_seconds',
  ],
  removePanel: (): void => {
    // Do nothing.
  },
  onExecuteQuery: (): void => {
    // Do nothing.
  },
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
    [
      { panelType: 'Table', active: true },
      { panelType: 'Graph', active: false },
    ].forEach((tc: { panelType: string; active: boolean }, i: number) => {
      const link = links.at(i);
      const className = tc.active ? 'active' : '';
      expect(link.prop('className')).toEqual(className);
      link.simulate('click');
      if (tc.active) {
        expect(results).toHaveLength(0);
      } else {
        expect(results).toHaveLength(1);
        expect(results[0].type).toEqual(tc.panelType.toLowerCase());
        results.pop();
      }
    });
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
    [
      { from: PanelType.Table, to: PanelType.Graph },
      { from: PanelType.Graph, to: PanelType.Table },
    ].forEach(({ from, to }: { from: PanelType; to: PanelType }) => {
      it(`${from} -> ${to} nulls out data`, () => {
        const props = {
          ...defaultProps,
          options: { ...defaultProps.options, type: from },
        };
        const panel = shallow(<Panel {...props} />);
        const instance: any = panel.instance();
        panel.setState({ data: 'somedata' });
        expect(panel.state('data')).toEqual('somedata');
        instance.handleChangeType(to);
        expect(panel.state('data')).toBeNull();
      });
    });
  });

  describe('when clicking on current mode', () => {
    [PanelType.Table, PanelType.Graph].forEach((mode: PanelType) => {
      it(`${mode} keeps data`, () => {
        const props = {
          ...defaultProps,
          options: { ...defaultProps.options, type: mode },
        };
        const panel = shallow(<Panel {...props} />);
        const instance: any = panel.instance();
        panel.setState({ data: 'somedata' });
        expect(panel.state('data')).toEqual('somedata');
        instance.handleChangeType(mode);
        expect(panel.state('data')).toEqual('somedata');
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
      //change query without executing
      panel.setProps({ options: { ...defaultProps.options, expr: newExpr } });
      expect(executeQuerySpy).toHaveBeenCalledTimes(0);
      //execute query implicitly with time change
      panel.setProps({ options: { ...defaultProps.options, expr: newExpr, endTime: 1575744840 } });
      expect(executeQuerySpy).toHaveBeenCalledTimes(1);
    });
  });
});
