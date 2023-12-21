import React, { Component } from 'react';
import { Button, ButtonGroup, Form, Input, InputGroup, InputGroupAddon } from 'reactstrap';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChartArea, faChartLine, faMinus, faPlus, faBarChart } from '@fortawesome/free-solid-svg-icons';
import TimeInput from './TimeInput';
import { formatDuration, parseDuration } from '../../utils';
import { GraphDisplayMode } from './Panel';

interface GraphControlsProps {
  range: number;
  endTime: number | null;
  useLocalTime: boolean;
  resolution: number | null;
  displayMode: GraphDisplayMode;
  isHeatmapData: boolean;
  showExemplars: boolean;
  onChangeRange: (range: number) => void;
  onChangeEndTime: (endTime: number | null) => void;
  onChangeResolution: (resolution: number | null) => void;
  onChangeShowExemplars: (show: boolean) => void;
  onChangeDisplayMode: (mode: GraphDisplayMode) => void;
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
    112 * 24 * 60 * 60,
    182 * 24 * 60 * 60,
    365 * 24 * 60 * 60,
    730 * 24 * 60 * 60,
  ].map((s) => s * 1000);

  onChangeRangeInput = (rangeText: string): void => {
    const range = parseDuration(rangeText);
    if (range === null) {
      this.changeRangeInput(this.props.range);
    } else {
      this.props.onChangeRange(range);
    }
  };

  changeRangeInput = (range: number): void => {
    if (this.rangeRef.current !== null) {
      this.rangeRef.current.value = formatDuration(range);
    }
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

  changeResolutionInput = (resolution: number | null): void => {
    if (this.resolutionRef.current !== null) {
      this.resolutionRef.current.value = resolution !== null ? resolution.toString() : '';
    }
  };

  componentDidUpdate(prevProps: GraphControlsProps): void {
    if (prevProps.range !== this.props.range) {
      this.changeRangeInput(this.props.range);
    }
    if (prevProps.resolution !== this.props.resolution) {
      this.changeResolutionInput(this.props.resolution);
    }
  }

  render(): JSX.Element {
    return (
      <Form inline className="graph-controls" onSubmit={(e) => e.preventDefault()}>
        <InputGroup className="range-input" size="sm">
          <InputGroupAddon addonType="prepend">
            <Button title="Decrease range" onClick={this.decreaseRange}>
              <FontAwesomeIcon icon={faMinus} fixedWidth />
            </Button>
          </InputGroupAddon>

          <Input
            defaultValue={formatDuration(this.props.range)}
            innerRef={this.rangeRef}
            onBlur={() => {
              if (this.rangeRef.current) {
                this.onChangeRangeInput(this.rangeRef.current.value);
              }
            }}
            onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) =>
              e.key === 'Enter' && this.rangeRef.current && this.onChangeRangeInput(this.rangeRef.current.value)
            }
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
            if (this.resolutionRef.current) {
              const res = parseInt(this.resolutionRef.current.value);
              this.props.onChangeResolution(res ? res : null);
            }
          }}
          bsSize="sm"
        />

        <ButtonGroup className="stacked-input" size="sm">
          <Button
            title="Show unstacked line graph"
            onClick={() => this.props.onChangeDisplayMode(GraphDisplayMode.Lines)}
            active={this.props.displayMode === GraphDisplayMode.Lines}
          >
            <FontAwesomeIcon icon={faChartLine} fixedWidth />
          </Button>
          <Button
            title="Show stacked graph"
            onClick={() => this.props.onChangeDisplayMode(GraphDisplayMode.Stacked)}
            active={this.props.displayMode === GraphDisplayMode.Stacked}
          >
            <FontAwesomeIcon icon={faChartArea} fixedWidth />
          </Button>
          {/* TODO: Consider replacing this button with a select dropdown in the future,
               to allow users to choose from multiple histogram series if available. */}
          {this.props.isHeatmapData && (
            <Button
              title="Show heatmap graph"
              onClick={() => this.props.onChangeDisplayMode(GraphDisplayMode.Heatmap)}
              active={this.props.displayMode === GraphDisplayMode.Heatmap}
            >
              <FontAwesomeIcon icon={faBarChart} fixedWidth />
            </Button>
          )}
        </ButtonGroup>

        <ButtonGroup className="show-exemplars" size="sm">
          {this.props.showExemplars ? (
            <Button title="Hide exemplars" onClick={() => this.props.onChangeShowExemplars(false)} active={true}>
              Hide Exemplars
            </Button>
          ) : (
            <Button title="Show exemplars" onClick={() => this.props.onChangeShowExemplars(true)} active={false}>
              Show Exemplars
            </Button>
          )}
        </ButtonGroup>
      </Form>
    );
  }
}

export default GraphControls;
