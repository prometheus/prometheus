import React, { Component } from 'react';
import {
  Button,
  ButtonGroup,
  Form,
  InputGroup,
  InputGroupAddon,
  Input,
} from 'reactstrap';

import moment from 'moment-timezone';

import 'tempusdominus-core';
import 'tempusdominus-bootstrap-4';
import '../node_modules/tempusdominus-bootstrap-4/build/css/tempusdominus-bootstrap-4.min.css';

import { dom, library } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faChevronLeft,
  faChevronRight,
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
  faClock,
  faCalendarCheck,
  faArrowUp,
  faArrowDown,
  faTimes,
} from '@fortawesome/free-solid-svg-icons';

library.add(
  faChevronLeft,
  faChevronRight,
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
  faClock,
  faCalendarCheck,
  faArrowUp,
  faArrowDown,
  faTimes,
);

// Sadly needed to also replace <i> within the date picker, since it's not a React component.
dom.watch();

class GraphControls extends Component {
  constructor(props) {
    super(props);

    this.state = {
      startDate: Date.now(),
    };

    this.rangeRef = React.createRef();
    this.endTimeRef = React.createRef();
    this.resolutionRef = React.createRef();
  }

  rangeUnits = {
    'y': 60 * 60 * 24 * 365,
    'w': 60 * 60 * 24 * 7,
    'd': 60 * 60 * 24,
    'h': 60 * 60,
    'm': 60,
    's': 1
  };

  rangeSteps = [
    '1s', '10s', '1m', '5m', '15m', '30m', '1h', '2h', '6h', '12h', '1d', '2d',
    '1w', '2w', '4w', '8w', '1y', '2y'
  ];

  parseRange(rangeText) {
    var rangeRE = new RegExp('^([0-9]+)([ywdhms]+)$');
    var matches = rangeText.match(rangeRE);
    if (!matches || matches.length !== 3) {
      return null;
    }
    var value = parseInt(matches[1]);
    var unit = matches[2];
    return value * this.rangeUnits[unit];
  }

  formatRange(range) {
    for (let unit of Object.keys(this.rangeUnits)) {
      if (range % this.rangeUnits[unit] === 0) {
        return (range / this.rangeUnits[unit]) + unit;
      }
    }
    return range + 's';
  }

  onChangeRangeInput = (rangeText) => {
    const range = this.parseRange(rangeText);
    if (range === null) {
      this.changeRangeInput(this.formatRange(this.props.range));
    } else {
      this.props.onChangeRange(this.parseRange(rangeText));
    }
  }

  changeRangeInput = (rangeText) => {
    this.rangeRef.current.value = rangeText;
  }

  increaseRange = (event) => {
    for (let range of this.rangeSteps) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range < rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
  }

  decreaseRange = (event) => {
    for (let range of this.rangeSteps.slice().reverse()) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range > rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
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
      <Form inline className="graph-controls" onSubmit={e => e.preventDefault()}>
        <InputGroup className="range-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease range" onClick={this.decreaseRange}><FontAwesomeIcon icon="minus" fixedWidth/></Button>
          </InputGroupAddon>

          {/* <Input value={this.state.rangeInput} onChange={(e) => this.changeRangeInput(e.target.value)}/> */}
          <Input
            defaultValue={this.formatRange(this.props.range)}
            innerRef={this.rangeRef}
            onBlur={() => this.onChangeRangeInput(this.rangeRef.current.value)}
          />

          <InputGroupAddon addonType="append">
            <Button title="Increase range" onClick={this.increaseRange}><FontAwesomeIcon icon="plus" fixedWidth/></Button>
          </InputGroupAddon>
        </InputGroup>

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

        <Input
          placeholder="Res. (s)"
          className="resolution-input"
          defaultValue={this.props.resolution !== null ? this.props.resolution : ''}
          innerRef={this.resolutionRef}
          onBlur={() => this.props.onChangeResolution(parseInt(this.resolutionRef.current.value))}
          bsSize="sm"
        />

        <ButtonGroup className="stacked-input" size="sm">
          <Button title="Show unstacked line graph" onClick={() => this.props.onChangeStacking(false)} active={!this.props.stacked}><FontAwesomeIcon icon="chart-line" fixedWidth/></Button>
          <Button title="Show stacked graph" onClick={() => this.props.onChangeStacking(true)} active={this.props.stacked}><FontAwesomeIcon icon="chart-area" fixedWidth/></Button>
        </ButtonGroup>
      </Form>
    );
  }
}

export default GraphControls;
