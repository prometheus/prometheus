import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import $ from 'jquery';
import moment from 'moment-timezone';
import TimeInput from './TimeInput';

const jqueryPlugins = $.fn as typeof $.fn & {
  datetimepicker: (...args: unknown[]) => JQuery<HTMLElement>;
};

const defaultProps = {
  time: 1572102237932,
  range: 60 * 60 * 7,
  placeholder: 'time input',
  onChangeTime: jest.fn(),
  useLocalTime: false,
};

describe('TimeInput', () => {
  let datetimepickerSpy: jest.SpyInstance;

  beforeEach(() => {
    defaultProps.onChangeTime.mockClear();
    datetimepickerSpy = jest.spyOn(jqueryPlugins, 'datetimepicker').mockReturnThis();
  });

  afterEach(() => {
    datetimepickerSpy.mockRestore();
  });

  it('initializes the picker and shifts or clears the selected time', () => {
    const { container } = render(<TimeInput {...defaultProps} />);

    expect(container.querySelector('.time-input')).toBeTruthy();
    expect(datetimepickerSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        format: 'YYYY-MM-DD HH:mm:ss',
        locale: 'en',
        timeZone: 'UTC',
        defaultDate: defaultProps.time,
      })
    );

    fireEvent.click(screen.getByTitle('Decrease time'));
    expect(defaultProps.onChangeTime).toHaveBeenLastCalledWith(defaultProps.time - defaultProps.range / 2);

    fireEvent.click(screen.getByTitle('Increase time'));
    expect(defaultProps.onChangeTime).toHaveBeenLastCalledWith(defaultProps.time + defaultProps.range / 2);

    fireEvent.click(screen.getByTitle('Clear time'));
    expect(defaultProps.onChangeTime).toHaveBeenLastCalledWith(null);
  });

  it('controls the picker through input events and prop updates', () => {
    const guessSpy = jest.spyOn(moment.tz, 'guess').mockReturnValue('Europe/Zurich');
    const { rerender, unmount } = render(<TimeInput {...defaultProps} />);
    const input = screen.getByPlaceholderText('time input');

    fireEvent.focus(input);
    expect(datetimepickerSpy).toHaveBeenLastCalledWith('show');
    fireEvent.blur(input);
    expect(datetimepickerSpy).toHaveBeenLastCalledWith('hide');
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(datetimepickerSpy).toHaveBeenLastCalledWith('hide');

    const updatedTime = defaultProps.time + 1000;
    rerender(<TimeInput {...defaultProps} time={updatedTime} useLocalTime />);
    const dateCall = datetimepickerSpy.mock.calls.find((call) => call[0] === 'date');
    expect(dateCall?.[1].valueOf()).toBe(updatedTime);
    expect(datetimepickerSpy).toHaveBeenCalledWith('options', {
      timeZone: 'Europe/Zurich',
      defaultDate: null,
    });

    const pickerEvent = $.Event('change.datetimepicker') as JQuery.TriggeredEvent & { date: moment.Moment };
    pickerEvent.date = moment(updatedTime + 1000);
    $(input).trigger(pickerEvent);
    expect(defaultProps.onChangeTime).toHaveBeenCalledWith(updatedTime + 1000);

    unmount();
    expect(datetimepickerSpy).toHaveBeenLastCalledWith('destroy');
    guessSpy.mockRestore();
  });

  it('hides the clear button when no time is selected', () => {
    render(<TimeInput {...defaultProps} time={null} />);

    expect(screen.queryByTitle('Clear time')).toBeNull();
  });
});
