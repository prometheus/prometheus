import * as React from 'react';
import { shallow } from 'enzyme';
import sinon from 'sinon';
import TimeInput from './TimeInput';
import { Button, InputGroup, InputGroupAddon, Input } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChevronLeft, faChevronRight, faTimes } from '@fortawesome/free-solid-svg-icons';

describe('TimeInput', () => {
  const timeInputProps = {
    time: 1572102237932,
    range: 60 * 60 * 7,
    placeholder: 'time input',
    onChangeTime: (): void => {
      // Do nothing.
    },
  };
  const timeInput = shallow(<TimeInput {...timeInputProps} />);
  it('renders the string "scalar"', () => {
    const inputGroup = timeInput.find(InputGroup);
    expect(inputGroup.prop('className')).toEqual('time-input');
    expect(inputGroup.prop('size')).toEqual('sm');
  });

  it('renders buttons to adjust time', () => {
    [
      {
        position: 'prepend',
        title: 'Decrease time',
        icon: faChevronLeft,
      },
      {
        position: 'append',
        title: 'Clear time',
        icon: faTimes,
      },
      {
        position: 'append',
        title: 'Increase time',
        icon: faChevronRight,
      },
    ].forEach(button => {
      const onChangeTime = sinon.spy();
      const timeInput = shallow(<TimeInput {...timeInputProps} onChangeTime={onChangeTime} />);
      const addon = timeInput.find(InputGroupAddon).filterWhere(addon => addon.prop('addonType') === button.position);
      const btn = addon.find(Button).filterWhere(btn => btn.prop('title') === button.title);
      const icon = btn.find(FontAwesomeIcon);
      expect(icon.prop('icon')).toEqual(button.icon);
      expect(icon.prop('fixedWidth')).toBe(true);
      btn.simulate('click');
      expect(onChangeTime.calledOnce).toBe(true);
    });
  });

  it('renders an Input', () => {
    const input = timeInput.find(Input);
    expect(input.prop('placeholder')).toEqual(timeInputProps.placeholder);
    expect(input.prop('innerRef')).toEqual({ current: null });
  });
});
