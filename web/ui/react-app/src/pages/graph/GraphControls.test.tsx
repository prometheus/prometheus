import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import GraphControls from './GraphControls';
import { GraphDisplayMode } from './Panel';

jest.mock('./TimeInput', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({ placeholder, onChangeTime }: { placeholder: string; onChangeTime: (time: number) => void }) =>
      React.createElement('button', { type: 'button', onClick: () => onChangeTime(5) }, placeholder),
  };
});

const createProps = () => ({
  range: 24 * 60 * 60 * 1000,
  endTime: 1572100217898,
  useLocalTime: false,
  resolution: 10,
  displayMode: GraphDisplayMode.Lines,
  isHeatmapData: false,
  showExemplars: false,
  onChangeRange: jest.fn(),
  onChangeEndTime: jest.fn(),
  onChangeResolution: jest.fn(),
  onChangeShowExemplars: jest.fn(),
  onChangeDisplayMode: jest.fn(),
});

describe('GraphControls', () => {
  it('changes range using step buttons and parsed input', () => {
    const props = createProps();
    render(<GraphControls {...props} />);
    const rangeInput = screen.getAllByRole('textbox')[0] as HTMLInputElement;

    expect(rangeInput.value).toBe('1d');
    fireEvent.click(screen.getByTitle('Decrease range'));
    expect(props.onChangeRange).toHaveBeenLastCalledWith(12 * 60 * 60 * 1000);

    fireEvent.click(screen.getByTitle('Increase range'));
    expect(props.onChangeRange).toHaveBeenLastCalledWith(2 * 24 * 60 * 60 * 1000);

    fireEvent.change(rangeInput, { target: { value: '2h' } });
    fireEvent.blur(rangeInput);
    expect(props.onChangeRange).toHaveBeenLastCalledWith(2 * 60 * 60 * 1000);

    fireEvent.change(rangeInput, { target: { value: 'invalid' } });
    fireEvent.blur(rangeInput);
    expect(rangeInput.value).toBe('1d');
  });

  it('reports end time and resolution changes', () => {
    const props = createProps();
    render(<GraphControls {...props} />);

    fireEvent.click(screen.getByRole('button', { name: 'End time' }));
    expect(props.onChangeEndTime).toHaveBeenCalledWith(5);

    const resolution = screen.getByPlaceholderText('Res. (s)') as HTMLInputElement;
    expect(resolution.value).toBe('10');
    fireEvent.change(resolution, { target: { value: '30' } });
    fireEvent.blur(resolution);
    expect(props.onChangeResolution).toHaveBeenLastCalledWith(30);

    fireEvent.change(resolution, { target: { value: '' } });
    fireEvent.blur(resolution);
    expect(props.onChangeResolution).toHaveBeenLastCalledWith(null);
  });

  it('selects graph display modes and exemplar visibility', () => {
    const props = createProps();
    const { rerender } = render(<GraphControls {...props} />);

    expect(screen.getByTitle('Show unstacked line graph').classList.contains('active')).toBe(true);
    fireEvent.click(screen.getByTitle('Show stacked graph'));
    expect(props.onChangeDisplayMode).toHaveBeenCalledWith(GraphDisplayMode.Stacked);
    expect(screen.queryByTitle('Show heatmap graph')).toBeNull();

    rerender(<GraphControls {...props} isHeatmapData />);
    fireEvent.click(screen.getByTitle('Show heatmap graph'));
    expect(props.onChangeDisplayMode).toHaveBeenLastCalledWith(GraphDisplayMode.Heatmap);

    fireEvent.click(screen.getByRole('button', { name: 'Show Exemplars' }));
    expect(props.onChangeShowExemplars).toHaveBeenCalledWith(true);
  });
});
