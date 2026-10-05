import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import Checkbox from './Checkbox';

describe('Checkbox', () => {
  it('associates its label with the checkbox and forwards input props', () => {
    const onChange = jest.fn();

    const { container } = render(
      <Checkbox id="alerts-enabled" checked={false} onChange={onChange} wrapperStyles={{ color: 'orange' }}>
        Alerts enabled
      </Checkbox>
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Alerts enabled' });
    expect(checkbox.getAttribute('id')).toBe('alerts-enabled');
    expect(checkbox.getAttribute('type')).toBe('checkbox');
    expect(checkbox.classList.contains('custom-control-input')).toBe(true);
    expect(container.firstElementChild?.classList.contains('custom-checkbox')).toBe(true);
    expect((container.firstElementChild as HTMLElement).style.color).toBe('orange');

    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});
