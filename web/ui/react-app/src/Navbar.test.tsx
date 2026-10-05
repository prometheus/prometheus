import * as React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import Navigation from './Navbar';

const renderNavigation = (consolesLink: string | null): ReturnType<typeof render> =>
  render(
    <MemoryRouter initialEntries={['/graph']}>
      <Navigation consolesLink={consolesLink} agentMode={false} />
      <Routes>
        <Route path="/status" element={<div data-testid="status-route" />} />
      </Routes>
    </MemoryRouter>
  );

describe('Navbar', () => {
  it.each([
    ['/path/consoles', '/path/consoles'],
    [null, null],
  ])('renders consoles link %s', (consolesLink, expectedHref) => {
    renderNavigation(consolesLink);

    const link = screen.queryByRole('link', { name: 'Consoles' });
    expect(link?.getAttribute('href') ?? null).toBe(expectedHref);
  });

  it('opens the status dropdown and navigates with React Router', async () => {
    renderNavigation(null);

    fireEvent.click(screen.getByText('Status'));
    fireEvent.click(screen.getByText('Runtime & Build Information'));

    await waitFor(() => expect(screen.getByTestId('status-route')).toBeTruthy());
  });
});
