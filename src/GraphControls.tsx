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

import TimeInput from './TimeInput';

library.add(
  faPlus,
  faMinus,
  faChartArea,
  faChartLine,
);

interface GraphControlsProps {
  range: number;
  endTime: number | null;
  resolution: number | null;
  stacked: boolean;

  onChangeRange: (range: number) => void;
  onChangeEndTime: (endTime: number | null) => void;
  onChangeResolution: (resolution: number) => void;
  onChangeStacking: (stacked: boolean) => void;
}

class GraphControls extends Component<GraphControlsProps> {
  private rangeRef = React.createRef<HTMLInputElement>();
  private resolutionRef = React.createRef<HTMLInputElement>();

  rangeUnits: {[unit: string]: number} = {
    'y': 60 * 60 * 24 * 365,
    'w': 60 * 60 * 24 * 7,
    'd': 60 * 60 * 24,
    'h': 60 * 60,
    'm': 60,
    's': 1
  }

  rangeSteps = [
    1,
    10,
    60,
    5*60,
    15*60,
    30*60,
    60*60,
    2*60*60,
    6*60*60,
    12*60*60,
    24*60*60,
    48*60*60,
    7*24*60*60,
    14*24*60*60,
    28*24*60*60,
    56*24*60*60,
    365*24*60*60,
    730*24*60*60,
  ]

  parseRange(rangeText: string): number | null {
    const rangeRE = new RegExp('^([0-9]+)([ywdhms]+)$');
    const matches = rangeText.match(rangeRE);
    if (!matches || matches.length !== 3) {
      return null;
    }
    const value = parseInt(matches[1]);
    const unit = matches[2];
    return value * this.rangeUnits[unit];
  }

  formatRange(range: number): string {
    for (let unit of Object.keys(this.rangeUnits)) {
      if (range % this.rangeUnits[unit] === 0) {
        return (range / this.rangeUnits[unit]) + unit;
      }
    }
    return range + 's';
  }

  onChangeRangeInput = (rangeText: string): void => {
    const range = this.parseRange(rangeText);
    if (range === null) {
      this.changeRangeInput(this.props.range);
    } else {
      this.props.onChangeRange(range);
    }
  }

  changeRangeInput = (range: number): void => {
    this.rangeRef.current!.value = this.formatRange(range);
  }

  increaseRange = (): void => {
    for (let range of this.rangeSteps) {
      if (this.props.range < range) {
        this.changeRangeInput(range);
        this.props.onChangeRange(range);
        return;
      }
    }
  }

  decreaseRange = (): void => {
    for (let range of this.rangeSteps.slice().reverse()) {
      if (this.props.range > range) {
        this.changeRangeInput(range);
        this.props.onChangeRange(range);
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
            onBlur={() => this.onChangeRangeInput(this.rangeRef.current!.value)}
          />

          <InputGroupAddon addonType="append">
            <Button title="Increase range" onClick={this.increaseRange}><FontAwesomeIcon icon="plus" fixedWidth/></Button>
          </InputGroupAddon>
        </InputGroup>

        <TimeInput endTime={this.props.endTime} range={this.props.range} onChangeEndTime={this.props.onChangeEndTime} />

        <Input
          placeholder="Res. (s)"
          className="resolution-input"
          defaultValue={this.props.resolution !== null ? this.props.resolution.toString() : ''}
          innerRef={this.resolutionRef}
          onBlur={() => this.props.onChangeResolution(parseInt(this.resolutionRef.current!.value))}
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
