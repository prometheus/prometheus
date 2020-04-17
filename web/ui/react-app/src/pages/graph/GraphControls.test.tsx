import * as React from 'react';
import { shallow } from 'enzyme';
import GraphControls from './GraphControls';
import { Button, ButtonGroup, Form, InputGroup, InputGroupAddon, Input } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faPlus, faMinus, faChartArea, faChartLine } from '@fortawesome/free-solid-svg-icons';
import TimeInput from './TimeInput';

const defaultGraphControlProps = {
  range: 60 * 60 * 24,
  endTime: 1572100217898,
  resolution: 10,
  stacked: false,

  onChangeRange: (): void => {
    // Do nothing.
  },
  onChangeEndTime: (): void => {
    // Do nothing.
  },
  onChangeResolution: (): void => {
    // Do nothing.
  },
  onChangeStacking: (): void => {
    // Do nothing.
  },
};

describe('GraphControls', () => {
  it('renders a form', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const form = controls.find(Form);
    expect(form).toHaveLength(1);
    expect(form.prop('className')).toEqual('graph-controls');
    expect(form.prop('inline')).toBe(true);
  });

  it('renders an Input Group for range', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const form = controls.find(InputGroup);
    expect(form).toHaveLength(1);
    expect(form.prop('className')).toEqual('range-input');
    expect(form.prop('size')).toBe('sm');
  });

  it('renders a decrease/increase range buttons', () => {
    [
      {
        position: 'prepend',
        title: 'Decrease range',
        icon: faMinus,
      },
      {
        position: 'append',
        title: 'Increase range',
        icon: faPlus,
      },
    ].forEach(testCase => {
      const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
      const addon = controls.find(InputGroupAddon).filterWhere(addon => addon.prop('addonType') === testCase.position);
      const button = addon.find(Button);
      const icon = button.find(FontAwesomeIcon);
      expect(button.prop('title')).toEqual(testCase.title);
      expect(icon).toHaveLength(1);
      expect(icon.prop('icon')).toEqual(testCase.icon);
      expect(icon.prop('fixedWidth')).toBe(true);
    });
  });

  it('renders an Input for range', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const form = controls.find(InputGroup);
    const input = form.find(Input);
    expect(input).toHaveLength(1);
    expect(input.prop('defaultValue')).toEqual('1d');
    expect(input.prop('innerRef')).toEqual({ current: null });
  });

  it('renders a TimeInput with props', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const timeInput = controls.find(TimeInput);
    expect(timeInput).toHaveLength(1);
    expect(timeInput.prop('time')).toEqual(1572100217898);
    expect(timeInput.prop('range')).toEqual(86400);
    expect(timeInput.prop('placeholder')).toEqual('End time');
  });

  it('renders a TimeInput with a callback', () => {
    const results: (number | null)[] = [];
    const onChange = (endTime: number | null): void => {
      results.push(endTime);
    };
    const controls = shallow(<GraphControls {...defaultGraphControlProps} onChangeEndTime={onChange} />);
    const timeInput = controls.find(TimeInput);
    const onChangeTime = timeInput.prop('onChangeTime');
    if (onChangeTime) {
      onChangeTime(5);
      expect(results).toHaveLength(1);
      expect(results[0]).toEqual(5);
      results.pop();
    } else {
      fail('Expected onChangeTime to be defined but it was not');
    }
  });

  it('renders a resolution Input with props', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const input = controls.find(Input).filterWhere(input => input.prop('className') === 'resolution-input');
    expect(input.prop('placeholder')).toEqual('Res. (s)');
    expect(input.prop('defaultValue')).toEqual('10');
    expect(input.prop('innerRef')).toEqual({ current: null });
    expect(input.prop('bsSize')).toEqual('sm');
  });

  it('renders a button group', () => {
    const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
    const group = controls.find(ButtonGroup);
    expect(group.prop('className')).toEqual('stacked-input');
    expect(group.prop('size')).toEqual('sm');
  });

  it('renders buttons inside the button group', () => {
    [
      {
        title: 'Show unstacked line graph',
        icon: faChartLine,
        active: true,
      },
      {
        title: 'Show stacked graph',
        icon: faChartArea,
        active: false,
      },
    ].forEach(testCase => {
      const controls = shallow(<GraphControls {...defaultGraphControlProps} />);
      const group = controls.find(ButtonGroup);
      const btn = group.find(Button).filterWhere(btn => btn.prop('title') === testCase.title);
      expect(btn.prop('active')).toEqual(testCase.active);
      const icon = btn.find(FontAwesomeIcon);
      expect(icon.prop('icon')).toEqual(testCase.icon);
    });
  });

  it('renders buttons with callbacks', () => {
    [
      {
        title: 'Show unstacked line graph',
        active: true,
      },
      {
        title: 'Show stacked graph',
        active: false,
      },
    ].forEach(testCase => {
      const results: boolean[] = [];
      const onChange = (stacked: boolean): void => {
        results.push(stacked);
      };
      const controls = shallow(<GraphControls {...defaultGraphControlProps} onChangeStacking={onChange} />);
      const group = controls.find(ButtonGroup);
      const btn = group.find(Button).filterWhere(btn => btn.prop('title') === testCase.title);
      const onClick = btn.prop('onClick');
      if (onClick) {
        onClick({} as React.MouseEvent);
        expect(results).toHaveLength(1);
        expect(results[0]).toBe(!testCase.active);
        results.pop();
      } else {
        fail('Expected onClick to be defined but it was not');
      }
    });
  });
});
