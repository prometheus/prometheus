import React, { Component } from 'react';
import { Button, ButtonGroup, Form, InputGroup, InputGroupAddon, Input } from 'reactstrap';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faPlus, faMinus, faChartArea, faChartLine } from '@fortawesome/free-solid-svg-icons';

import TimeInput from './TimeInput';
import { parseRange, formatRange } from '../../utils';

interface GraphControlsProps {
  range: number;
  endTime: number | null;
  useLocalTime: boolean;
  resolution: number | null;
  stacked: boolean;

  onChangeRange: (range: number) => void;
  onChangeEndTime: (endTime: number | null) => void;
  onChangeResolution: (resolution: number | null) => void;
  onChangeStacking: (stacked: boolean) => void;
}

class GraphControls extends Component<GraphControlsProps> {
  private rangeRef = React.createRef<HTMLInputElement>();
  private resolutionRef = React.createRef<HTMLInputElement>();

  rangeSteps = [
    1,
    10,
    60,
    5 * 60,
    15 * 60,
    30 * 60,
    60 * 60,
    2 * 60 * 60,
    6 * 60 * 60,
    12 * 60 * 60,
    24 * 60 * 60,
    48 * 60 * 60,
    7 * 24 * 60 * 60,
    14 * 24 * 60 * 60,
    28 * 24 * 60 * 60,
    56 * 24 * 60 * 60,
    365 * 24 * 60 * 60,
    730 * 24 * 60 * 60,
  ];

  onChangeRangeInput = (rangeText: string): void => {
    const range = parseRange(rangeText);
    if (range === null) {
      this.changeRangeInput(this.props.range);
    } else {
      this.props.onChangeRange(range);
    }
  };

  changeRangeInput = (range: number): void => {
    this.rangeRef.current!.value = formatRange(range);
  };

  increaseRange = (): void => {
    for (const range of this.rangeSteps) {
      if (this.props.range < range) {
        this.changeRangeInput(range);
        this.props.onChangeRange(range);
        return;
      }
    }
  };

  decreaseRange = (): void => {
    for (const range of this.rangeSteps.slice().reverse()) {
      if (this.props.range > range) {
        this.changeRangeInput(range);
        this.props.onChangeRange(range);
        return;
      }
    }
  };

  componentDidUpdate(prevProps: GraphControlsProps) {
    if (prevProps.range !== this.props.range) {
      this.changeRangeInput(this.props.range);
    }
    if (prevProps.resolution !== this.props.resolution) {
      this.resolutionRef.current!.value = this.props.resolution !== null ? this.props.resolution.toString() : '';
    }
  }

  render() {
    return (
      <Form inline className="graph-controls" onSubmit={e => e.preventDefault()}>
        <InputGroup className="range-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease range" onClick={this.decreaseRange}>
              <FontAwesomeIcon icon={faMinus} fixedWidth />
            </Button>
          </InputGroupAddon>

          <Input
            defaultValue={formatRange(this.props.range)}
            innerRef={this.rangeRef}
            onBlur={() => this.onChangeRangeInput(this.rangeRef.current!.value)}
          />

          <InputGroupAddon addonType="append">
            <Button title="Increase range" onClick={this.increaseRange}>
              <FontAwesomeIcon icon={faPlus} fixedWidth />
            </Button>
          </InputGroupAddon>
        </InputGroup>

        <TimeInput
          time={this.props.endTime}
          useLocalTime={this.props.useLocalTime}
          range={this.props.range}
          placeholder="End time"
          onChangeTime={this.props.onChangeEndTime}
        />

        <Input
          placeholder="Res. (s)"
          className="resolution-input"
          defaultValue={this.props.resolution !== null ? this.props.resolution.toString() : ''}
          innerRef={this.resolutionRef}
          onBlur={() => {
            const res = parseInt(this.resolutionRef.current!.value);
            this.props.onChangeResolution(res ? res : null);
          }}
          bsSize="sm"
        />

        <ButtonGroup className="stacked-input" size="sm">
          <Button
            title="Show unstacked line graph"
            onClick={() => this.props.onChangeStacking(false)}
            active={!this.props.stacked}
          >
            <FontAwesomeIcon icon={faChartLine} fixedWidth />
          </Button>
          <Button title="Show stacked graph" onClick={() => this.props.onChangeStacking(true)} active={this.props.stacked}>
            <FontAwesomeIcon icon={faChartArea} fixedWidth />
          </Button>
        </ButtonGroup>
      </Form>
    );
  }
}

export default GraphControls;
