import * as React from 'react';
import { shallow } from 'enzyme';
import Legend from './Legend';
import SeriesName from './SeriesName';

describe('Legend', () => {
  describe('regardless of series', () => {
    it('renders a table', () => {
      const legend = shallow(<Legend series={[]} />);
      expect(legend.type()).toEqual('table');
      expect(legend.prop('className')).toEqual('graph-legend');
      const tbody = legend.children();
      expect(tbody.type()).toEqual('tbody');
    });
  });
  describe('when series is empty', () => {
    it('renders props as empty legend table', () => {
      const legend = shallow(<Legend series={[]} />);
      const tbody = legend.children();
      expect(tbody.children()).toHaveLength(0);
    });
  });

  describe('when series has one element', () => {
    const legendProps = {
      series: [
        {
          index: 1,
          color: 'red',
          labels: {
            __name__: 'metric_name',
            label1: 'value_1',
            labeln: 'value_n',
          },
        },
      ],
    };
    it('renders a row of the one series', () => {
      const legend = shallow(<Legend {...legendProps} />);
      const tbody = legend.children();
      expect(tbody.children()).toHaveLength(1);
      const row = tbody.find('tr');
      expect(row.prop('className')).toEqual('legend-item');
    });
    it('renders a legend swatch', () => {
      const legend = shallow(<Legend {...legendProps} />);
      const tbody = legend.children();
      const row = tbody.find('tr');
      const swatch = row.childAt(0);
      expect(swatch.type()).toEqual('td');
      expect(swatch.children().prop('className')).toEqual('legend-swatch');
      expect(swatch.children().prop('style')).toEqual({
        backgroundColor: 'red',
      });
    });
    it('renders a series name', () => {
      const legend = shallow(<Legend {...legendProps} />);
      const tbody = legend.children();
      const row = tbody.find('tr');
      const series = row.childAt(1);
      expect(series.type()).toEqual('td');
      const seriesName = series.find(SeriesName);
      expect(seriesName).toHaveLength(1);
      expect(seriesName.prop('labels')).toEqual(legendProps.series[0].labels);
      expect(seriesName.prop('format')).toBe(true);
    });
  });

  describe('when series has _n_ elements', () => {
    const range = Array.from(Array(20).keys());
    const legendProps = {
      series: range.map(i => ({
        index: i,
        color: 'red',
        labels: {
          __name__: `metric_name_${i}`,
          label1: 'value_1',
          labeln: 'value_n',
        },
      })),
    };
    it('renders _n_ rows', () => {
      const legend = shallow(<Legend {...legendProps} />);
      const tbody = legend.children();
      expect(tbody.children()).toHaveLength(20);
      const rows = tbody.find('tr');
      rows.forEach(row => {
        expect(row.prop('className')).toEqual('legend-item');
        expect(row.find(SeriesName)).toHaveLength(1);
      });
    });
  });
});
