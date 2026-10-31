import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { ToggleMoreLess } from './ToggleMoreLess';

describe('ToggleMoreLess', () => {
  it('renders the controlled state and reports clicks', () => {
    const event = jest.fn();
    const { rerender } = render(<ToggleMoreLess event={event} showMore={false} />);

    const button = screen.getByRole('button', { name: 'show more' });
    expect(button.classList.contains('btn-primary')).toBe(true);
    expect(button.classList.contains('btn-xs')).toBe(true);

    fireEvent.click(button);
    expect(event).toHaveBeenCalledTimes(1);

    rerender(<ToggleMoreLess event={event} showMore />);
    expect(screen.getByRole('button', { name: 'show less' })).toBeTruthy();
  });
});
