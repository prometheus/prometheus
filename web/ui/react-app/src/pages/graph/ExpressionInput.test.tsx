import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import ExpressionInput from './ExpressionInput';

const defaultProps = {
  value: 'node_cpu',
  queryHistory: [],
  metricNames: [],
  executeQuery: jest.fn(),
  onExpressionChange: jest.fn(),
  loading: false,
  enableAutocomplete: true,
  enableHighlighting: true,
  enableLinter: true,
};

describe('ExpressionInput', () => {
  const getSelection = document.getSelection;

  beforeAll(() => {
    document.getSelection = () =>
      ({
        addRange: jest.fn(),
        collapse: jest.fn(),
        removeAllRanges: jest.fn(),
      }) as unknown as Selection;
  });

  afterAll(() => {
    document.getSelection = getSelection;
  });

  beforeEach(() => {
    defaultProps.executeQuery.mockClear();
    defaultProps.onExpressionChange.mockClear();
  });

  it('renders a CodeMirror expression and executes the query', () => {
    const { container } = render(<ExpressionInput {...defaultProps} />);

    expect(container.querySelector('.expression-input')).toBeTruthy();
    expect(container.querySelector('.cm-expression-input')?.textContent).toContain('node_cpu');
    expect(container.querySelector('[data-icon="magnifying-glass"]')).toBeTruthy();

    const execute = screen.getByRole('button', { name: 'Execute' });
    expect(execute.classList.contains('btn-primary')).toBe(true);
    fireEvent.click(execute);
    expect(defaultProps.executeQuery).toHaveBeenCalledTimes(1);
  });

  it('shows a spinner while loading', () => {
    const { container } = render(<ExpressionInput {...defaultProps} loading />);

    expect(container.querySelector('[data-icon="spinner"]')).toBeTruthy();
    expect(container.querySelector('[data-icon="magnifying-glass"]')).toBeNull();
  });
});
