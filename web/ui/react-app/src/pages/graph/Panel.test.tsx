import React, { FC, useState } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import Panel, { GraphDisplayMode, PanelOptions, PanelType } from './Panel';

jest.mock('./ExpressionInput', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({
      value,
      onExpressionChange,
      executeQuery,
      loading,
    }: {
      value: string;
      onExpressionChange: (value: string) => void;
      executeQuery: () => void;
      loading: boolean;
    }) =>
      React.createElement(
        'div',
        { 'data-testid': 'expression-input', 'data-loading': loading },
        React.createElement('input', {
          'aria-label': 'Expression',
          value,
          onChange: (event: React.ChangeEvent<HTMLInputElement>) => onExpressionChange(event.target.value),
        }),
        React.createElement('button', { type: 'button', onClick: executeQuery }, 'Execute')
      ),
  };
});

jest.mock('./TimeInput', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({
      time,
      range,
      placeholder,
      onChangeTime,
    }: {
      time: number | null;
      range: number;
      placeholder: string;
      onChangeTime: (time: number) => void;
    }) =>
      React.createElement(
        'button',
        {
          type: 'button',
          'data-testid': 'time-input',
          'data-time': time,
          'data-range': range,
          onClick: () => onChangeTime(1575744840000),
        },
        placeholder
      ),
  };
});

jest.mock('./DataTable', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({ data }: { data: { resultType?: string } | null }) =>
      React.createElement('div', { 'data-testid': 'data-table', 'data-state': data?.resultType || 'null' }),
  };
});

jest.mock('./GraphControls', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    default: ({
      range,
      endTime,
      resolution,
      displayMode,
    }: {
      range: number;
      endTime: number | null;
      resolution: number | null;
      displayMode: GraphDisplayMode;
    }) =>
      React.createElement('div', {
        'data-testid': 'graph-controls',
        'data-range': range,
        'data-end-time': endTime,
        'data-resolution': resolution,
        'data-display-mode': displayMode,
      }),
  };
});

jest.mock('./GraphTabContent', () => {
  const React = jest.requireActual('react');
  return {
    __esModule: true,
    GraphTabContent: ({ data, displayMode }: { data: { resultType?: string } | null; displayMode: GraphDisplayMode }) =>
      React.createElement('div', {
        'data-testid': 'graph-content',
        'data-state': data?.resultType || 'null',
        'data-display-mode': displayMode,
      }),
  };
});

const defaultOptions: PanelOptions = {
  expr: 'prometheus_engine',
  type: PanelType.Table,
  range: 10,
  endTime: 1572100217898,
  resolution: 28,
  displayMode: GraphDisplayMode.Lines,
  showExemplars: false,
};

interface HarnessProps {
  initialOptions?: PanelOptions;
  onExecuteQuery?: (query: string) => void;
  removePanel?: () => void;
}

const PanelHarness: FC<HarnessProps> = ({
  initialOptions = defaultOptions,
  onExecuteQuery = jest.fn(),
  removePanel = jest.fn(),
}) => {
  const [options, setOptions] = useState(initialOptions);
  return (
    <>
      <output data-testid="panel-options">{JSON.stringify(options)}</output>
      <Panel
        options={options}
        onOptionsChanged={setOptions}
        useLocalTime={false}
        pastQueries={[]}
        metricNames={['prometheus_engine']}
        removePanel={removePanel}
        onExecuteQuery={onExecuteQuery}
        pathPrefix=""
        enableAutocomplete
        enableHighlighting
        enableLinter
        id="panel"
      />
    </>
  );
};

const vectorResponse = JSON.stringify({
  status: 'success',
  data: { resultType: 'vector', result: [{ metric: {}, value: [1572100217, '1'] }] },
});
const matrixResponse = JSON.stringify({ status: 'success', data: { resultType: 'matrix', result: [] } });

const firstRequestURL = (): URL => {
  const request = fetchMock.mock.calls[0]?.[0];
  if (!request) {
    throw new Error('expected a fetch request');
  }
  return new URL(request.toString(), 'http://localhost');
};

describe('Panel', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
    fetchMock.mockResponse(vectorResponse);
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('executes the table query and renders returned data', async () => {
    const onExecuteQuery = jest.fn();
    render(<PanelHarness onExecuteQuery={onExecuteQuery} />);

    await waitFor(() => expect(screen.getByTestId('data-table').getAttribute('data-state')).toBe('vector'));
    expect(onExecuteQuery).toHaveBeenCalledWith('prometheus_engine');
    const requestURL = firstRequestURL();
    expect(requestURL.pathname).toBe('/api/v1/query');
    expect(requestURL.searchParams.get('query')).toBe('prometheus_engine');
    expect(requestURL.searchParams.get('time')).toBe('1572100217.898');
    expect(screen.getByTestId('time-input').textContent).toBe('Evaluation time');
  });

  it('clears stale data when switching modes and ignores the active mode', async () => {
    let resolveGraphQuery: (body: string) => void = () => undefined;
    fetchMock.resetMocks();
    fetchMock.mockResponseOnce(vectorResponse);
    fetchMock.mockResponseOnce(() => new Promise((resolve) => (resolveGraphQuery = resolve)));
    render(<PanelHarness />);
    await waitFor(() => expect(screen.getByTestId('data-table').getAttribute('data-state')).toBe('vector'));

    fireEvent.click(screen.getByText('Table'));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('data-table').getAttribute('data-state')).toBe('vector');

    fireEvent.click(screen.getByText('Graph'));
    expect(screen.getByTestId('graph-content').getAttribute('data-state')).toBe('null');
    expect(JSON.parse(screen.getByTestId('panel-options').textContent || '').type).toBe(PanelType.Graph);

    await act(async () => resolveGraphQuery(matrixResponse));
    await waitFor(() => expect(screen.getByTestId('graph-content').getAttribute('data-state')).toBe('matrix'));
    expect(screen.getByTestId('graph-controls').getAttribute('data-display-mode')).toBe(GraphDisplayMode.Lines);
  });

  it('executes the edited expression after a debounced time change', async () => {
    jest.useFakeTimers();
    const onExecuteQuery = jest.fn();
    render(<PanelHarness initialOptions={{ ...defaultOptions, expr: '' }} onExecuteQuery={onExecuteQuery} />);
    expect(fetchMock).not.toHaveBeenCalled();

    fireEvent.change(screen.getByRole('textbox', { name: 'Expression' }), {
      target: { value: 'time() - time()' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Evaluation time' }));
    act(() => jest.advanceTimersByTime(250));
    await act(async () => undefined);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const requestURL = firstRequestURL();
    expect(requestURL.searchParams.get('query')).toBe('time() - time()');
    expect(requestURL.searchParams.get('time')).toBe('1575744840');
    expect(JSON.parse(screen.getByTestId('panel-options').textContent || '').expr).toBe('time() - time()');
  });

  it('removes the panel through its visible control', () => {
    const removePanel = jest.fn();
    render(<PanelHarness initialOptions={{ ...defaultOptions, expr: '' }} removePanel={removePanel} />);

    fireEvent.click(screen.getByRole('button', { name: 'Remove Panel' }));
    expect(removePanel).toHaveBeenCalledTimes(1);
  });
});
