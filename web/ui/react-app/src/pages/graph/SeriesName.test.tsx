import * as React from 'react';
import { shallow, ShallowWrapper } from 'enzyme';
import SeriesName from './SeriesName';

describe('SeriesName', () => {
  describe('with labels=null', () => {
    const seriesNameProps = {
      labels: null,
      format: false,
    };
    const seriesName = shallow(<SeriesName {...seriesNameProps} />);
    it('renders the string "scalar"', () => {
      expect(seriesName.text()).toEqual('scalar');
    });
  });

  describe('with labels defined and format false', () => {
    const seriesNameProps = {
      labels: {
        __name__: 'metric_name',
        label1: 'value_1',
        label2: 'value_2',
        label3: 'value_3',
      },
      format: false,
    };
    const seriesName = shallow(<SeriesName {...seriesNameProps} />);
    it('renders the series name as a string', () => {
      expect(seriesName.text()).toEqual('metric_name{label1="value_1", label2="value_2", label3="value_3"}');
    });
  });

  describe('with labels defined and format true', () => {
    const seriesNameProps = {
      labels: {
        __name__: 'metric_name',
        label1: 'value_1',
        label2: 'value_2',
        label3: 'value_3',
      },
      format: true,
    };
    const seriesName = shallow(<SeriesName {...seriesNameProps} />);
    it('renders the series name as a series of spans', () => {
      expect(seriesName.children()).toHaveLength(6);
      const testCases = [
        { name: 'metric_name', className: 'legend-metric-name' },
        { name: '{', className: 'legend-label-brace' },
        { name: 'label1', value: 'value_1', className: 'legend-label-name' },
        { name: 'label2', value: 'value_2', className: 'legend-label-name' },
        { name: 'label3', value: 'value_3', className: 'legend-label-name' },
        { name: '}', className: 'legend-label-brace' },
      ];

      const testLabelContainerElement = (text: string, expectedText: string, container: ShallowWrapper) => {
        expect(text).toEqual(expectedText);
        expect(container.prop('className')).toEqual('legend-label-container');
        expect(container.childAt(0).prop('className')).toEqual('legend-label-name');
        expect(container.childAt(2).prop('className')).toEqual('legend-label-value');
      };

      testCases.forEach((tc, i) => {
        const child = seriesName.childAt(i);
        const firstChildElement = child.childAt(0);
        const firstChildElementClass = firstChildElement.prop('className');
        const text = child
          .children()
          .map((ch) => ch.text())
          .join('');
        switch (child.children().length) {
          case 1:
            switch (firstChildElementClass) {
              case 'legend-label-container':
                testLabelContainerElement(text, `${tc.name}="${tc.value}"`, firstChildElement);
                break;
              default:
                expect(text).toEqual(tc.name);
                expect(child.prop('className')).toEqual(tc.className);
            }
            break;
          case 2:
            const container = child.childAt(1);
            testLabelContainerElement(text, `, ${tc.name}="${tc.value}"`, container);
            break;
          default:
            fail('incorrect number of children: ' + child.children().length);
        }
      });
    });
  });
});
