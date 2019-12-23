import * as React from 'react';
import { shallow } from 'enzyme';
import { Alert } from 'reactstrap';
import { GraphTabContent } from './GraphTabContent';

describe('GraphTabContent', () => {
  it('renders an alert if data result type is different than "matrix"', () => {
    const props: any = {
      data: { resultType: 'invalid', result: [{}] },
      stacked: false,
      queryParams: {
        startTime: 1572100210000,
        endTime: 1572100217898,
        resolution: 10,
      },
      color: 'danger',
      children: `Query result is of wrong type '`,
    };
    const graph = shallow(<GraphTabContent {...props} />);
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
    const graph = shallow(<GraphTabContent {...props} />);
    const alert = graph.find(Alert);
    expect(alert.prop('color')).toEqual(props.color);
    expect(alert.childAt(0).text()).toEqual(props.children);
  });
});
