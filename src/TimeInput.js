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

class TimeInput extends Component {
  constructor(props) {
    super(props);

    this.endTimeRef = React.createRef();
  }

  getBaseEndTime = () => {
    return this.props.endTime || moment();
  }

  increaseEndTime = (event) => {
    const endTime = moment(this.getBaseEndTime() + this.props.range*1000/2);
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', endTime);
  }

  decreaseEndTime = (event) => {
    const endTime = moment(this.getBaseEndTime() - this.props.range*1000/2);
    this.props.onChangeEndTime(endTime);
    this.$endTime.datetimepicker('date', endTime);
  }

  clearEndTime = (event) => {
    this.props.onChangeEndTime(null);
    this.$endTime.datetimepicker('date', null);
  }

  componentDidMount() {
    this.$endTime = window.$(this.endTimeRef.current);

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
      format: 'YYYY-MM-DD HH:mm:ss',
      locale: 'en',
      timeZone: 'UTC',
      defaultDate: this.props.endTime,
    });

    this.$endTime.on('change.datetimepicker', e => {
      console.log("CHANGE", e)
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
