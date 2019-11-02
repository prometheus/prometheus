import React from 'react';
import { shallow } from 'enzyme';
import StateIndicator from './StateIndicator';
import { Alert } from 'reactstrap';

describe('StateIndicator', () => {
  const defaultProps = {
    health: 'up',
  };
  const indicator = shallow(<StateIndicator {...defaultProps} />);
  it('renders an alert', () => {
    const alert = indicator.find(Alert);
    expect(alert.prop('color')).toEqual('success');
    expect(alert.prop('className')).toEqual('message');
    expect(alert.children().text()).toEqual('UP');
  });
  it('renders a red alert if down', () => {
    const props = {
      health: 'down',
    };
    const indicator = shallow(<StateIndicator {...props} />);
    const alert = indicator.find(Alert);
    expect(alert.prop('color')).toEqual('danger');
    expect(alert.prop('className')).toEqual('message');
    expect(alert.children().text()).toEqual('DOWN');
  });
  it('renders a yellow alert if unknown', () => {
    const props = {
      health: 'unknown',
    };
    const indicator = shallow(<StateIndicator {...props} />);
    const alert = indicator.find(Alert);
    expect(alert.prop('color')).toEqual('warning');
    expect(alert.prop('className')).toEqual('message');
    expect(alert.children().text()).toEqual('UNKNOWN');
  });

  it('renders message if message is passed in', () => {
    const props = {
      health: 'down',
      message: 'Returned 404',
    };
    const indicator = shallow(<StateIndicator {...props} />);
    const alert = indicator.find(Alert);
    expect(alert.prop('color')).toEqual('danger');
    expect(alert.prop('className')).toEqual('message');
    expect(alert.children().text()).toEqual('Returned 404');
  });
});
