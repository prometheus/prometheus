import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import PanelList, { PanelListContent } from './PanelList';
import { PanelDefaultOptions } from './Panel';

jest.mock('./Panel', () => {
  const React = jest.requireActual('react');
  const actual = jest.requireActual('./Panel');
  return {
    __esModule: true,
    ...actual,
    default: ({ id }: { id: string }) => React.createElement('div', { 'data-testid': 'panel', 'data-panel-id': id }),
  };
});

const contentProps = {
  metrics: [],
  useLocalTime: false,
  queryHistoryEnabled: false,
  enableAutocomplete: true,
  enableHighlighting: true,
  enableLinter: true,
};

describe('PanelList', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
    localStorage.clear();
    window.history.replaceState({}, '', '/graph');
  });

  afterEach(() => {
    window.onpopstate = null;
    localStorage.clear();
  });

  it('renders the persisted configuration defaults', () => {
    const now = new Date().getTime() / 1000;
    fetchMock.mockResponses(
      JSON.stringify({ status: 'success', data: ['up'] }),
      JSON.stringify({ status: 'success', data: { result: [now] } })
    );

    render(<PanelList />);

    expect((screen.getByRole('checkbox', { name: 'Use local time' }) as HTMLInputElement).checked).toBe(false);
    expect((screen.getByRole('checkbox', { name: 'Enable query history' }) as HTMLInputElement).checked).toBe(false);
    expect((screen.getByRole('checkbox', { name: 'Enable autocomplete' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'Enable highlighting' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'Enable linter' }) as HTMLInputElement).checked).toBe(true);
  });

  it('renders existing panels and adds another panel', () => {
    render(
      <PanelListContent {...contentProps} panels={[{ id: 'existing-panel', key: '0', options: PanelDefaultOptions }]} />
    );

    expect(screen.getAllByTestId('panel')).toHaveLength(1);
    expect(screen.getByTestId('panel').getAttribute('data-panel-id')).toBe('existing-panel');

    fireEvent.click(screen.getByRole('button', { name: 'Add Panel' }));
    expect(screen.getAllByTestId('panel')).toHaveLength(2);
  });

  it('creates an initial panel when the URL has none', () => {
    render(<PanelListContent {...contentProps} panels={[]} />);

    expect(screen.getAllByTestId('panel')).toHaveLength(1);
  });
});
