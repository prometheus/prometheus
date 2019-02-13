import React, { Component } from 'react';
import {
  Button,
  ButtonGroup,
  Form,
  InputGroup,
  InputGroupAddon,
  Input,
} from 'reactstrap';

import 'tempusdominus-core';
import 'tempusdominus-bootstrap-4';
import '../node_modules/tempusdominus-bootstrap-4/build/css/tempusdominus-bootstrap-4.min.css';

import { library } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
} from '@fortawesome/free-solid-svg-icons';

import TimeInput from './TimeInput.js';

library.add(
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
);

class GraphControls extends Component {
  constructor(props) {
    super(props);

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
  }

  rangeSteps = [
    '1s', '10s', '1m', '5m', '15m', '30m', '1h', '2h', '6h', '12h', '1d', '2d',
    '1w', '2w', '4w', '8w', '1y', '2y'
  ]

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

  increaseRange = () => {
    for (let range of this.rangeSteps) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range < rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
  }

  decreaseRange = () => {
    for (let range of this.rangeSteps.slice().reverse()) {
      let rangeSeconds = this.parseRange(range);
      if (this.props.range > rangeSeconds) {
        this.changeRangeInput(range);
        this.props.onChangeRange(rangeSeconds);
        return;
      }
    }
  }

  render() {
    return (
      <Form inline className="graph-controls" onSubmit={e => e.preventDefault()}>
        <InputGroup className="range-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease range" onClick={this.decreaseRange}><FontAwesomeIcon icon="minus" fixedWidth/></Button>
          </InputGroupAddon>

          <Input
            defaultValue={this.formatRange(this.props.range)}
            innerRef={this.rangeRef}
            onBlur={() => this.onChangeRangeInput(this.rangeRef.current.value)}
          />

          <InputGroupAddon addonType="append">
            <Button title="Increase range" onClick={this.increaseRange}><FontAwesomeIcon icon="plus" fixedWidth/></Button>
          </InputGroupAddon>
        </InputGroup>

        <TimeInput endTime={this.props.endTime} range={this.props.range} onChangeEndTime={this.props.onChangeEndTime} />

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
