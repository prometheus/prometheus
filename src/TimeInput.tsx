import $ from 'jquery';
import React, { Component } from 'react';
import { Button, InputGroup, InputGroupAddon, Input } from 'reactstrap';

import moment from 'moment-timezone';

import 'tempusdominus-core';
import 'tempusdominus-bootstrap-4';
import '../node_modules/tempusdominus-bootstrap-4/build/css/tempusdominus-bootstrap-4.min.css';

import { dom, library } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faChevronLeft,
  faChevronRight,
  faCalendarCheck,
  faArrowUp,
  faArrowDown,
  faTimes,
} from '@fortawesome/free-solid-svg-icons';

library.add(
  faChevronLeft,
  faChevronRight,
  faCalendarCheck,
  faArrowUp,
  faArrowDown,
  faTimes,
);

// Sadly needed to also replace <i> within the date picker, since it's not a React component.
dom.watch();

interface TimeInputProps {
  endTime: number | null;
  range: number;

  onChangeEndTime: (endTime: number | null) => void;
}

class TimeInput extends Component<TimeInputProps> {
  private endTimeRef = React.createRef<HTMLInputElement>();
  private $endTime: any | null = null;

  getBaseEndTime = (): number => {
    return this.props.endTime || moment().valueOf();
  }

  increaseEndTime = (): void => {
    const endTime = this.getBaseEndTime() + this.props.range*1000/2;
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', moment(endTime));
  }

  decreaseEndTime = (): void => {
    const endTime = this.getBaseEndTime() - this.props.range*1000/2;
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', moment(endTime));
  }

  clearEndTime = (): void => {
    this.props.onChangeEndTime(null);
    this.$endTime.datetimepicker('date', null);
  }

  componentDidMount() {
    this.$endTime = $(this.endTimeRef.current!);

    this.$endTime.datetimepicker({
      icons: {
        today: 'fas fa-calendar-check',
      },
      buttons: {
        //showClear: true,
        showClose: true,
        showToday: true,
      },
      sideBySide: true,
      format: 'YYYY-MM-DD HH:mm',
      locale: 'en',
      timeZone: 'UTC',
      defaultDate: this.props.endTime,
    });

    this.$endTime.on('change.datetimepicker', (e: any) => {
      if (e.date) {
        this.props.onChangeEndTime(e.date);
      } else {
        this.$endTime.datetimepicker('date', e.target.value);
      }
    });
  }

  componentWillUnmount() {
    this.$endTime.datetimepicker('destroy');
  }

  render() {
    return (
      <InputGroup className="endtime-input" size="sm">
        <InputGroupAddon addonType="prepend">
          <Button title="Decrease end time" onClick={this.decreaseEndTime}><FontAwesomeIcon icon="chevron-left" fixedWidth/></Button>
        </InputGroupAddon>

        <Input
          placeholder="End time"
          // value={this.props.endTime ? this.props.endTime : ''}
          innerRef={this.endTimeRef}
          // onChange={this.props.onChangeEndTime}
          onFocus={() => this.$endTime.datetimepicker('show')}
          onBlur={() => this.$endTime.datetimepicker('hide')}
          onKeyDown={(e) => ['Escape', 'Enter'].includes(e.key) && this.$endTime.datetimepicker('hide')}
        />

        {/* CAUTION: While the datetimepicker also has an option to show a 'clear' button,
            that functionality is broken, so we create an external solution instead. */}
        {this.props.endTime &&
          <InputGroupAddon addonType="append">
            <Button className="clear-endtime-btn" title="Clear end time" onClick={this.clearEndTime}><FontAwesomeIcon icon="times" fixedWidth/></Button>
          </InputGroupAddon>
        }

        <InputGroupAddon addonType="append">
          <Button title="Increase end time" onClick={this.increaseEndTime}><FontAwesomeIcon icon="chevron-right" fixedWidth/></Button>
        </InputGroupAddon>
      </InputGroup>
    );
  }
}

export default TimeInput;
