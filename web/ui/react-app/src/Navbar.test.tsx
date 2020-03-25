import * as React from 'react';
import { mount } from 'enzyme';
import Navigation from './Navbar';
import { NavItem, NavLink } from 'reactstrap';
import { GlobalVarsProvider } from './hooks/useGlobalVars';

describe('Navbar should contain console Link', () => {
  it('with non-empty consoleslink', () => {
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_PATH_PREFIX = '/path/prefix';
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_CONSOLES_LINK = '/path/consoles';
    const app = mount(
      <GlobalVarsProvider>
        <Navigation />
      </GlobalVarsProvider>
    );
    expect(
      app.contains(
        <NavItem>
          <NavLink href="/path/consoles">Consoles</NavLink>
        </NavItem>
      )
    ).toBeTruthy();
  });
});

describe('Navbar should not contain consoles link', () => {
  it('with empty string in consolesLink', () => {
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_PATH_PREFIX = '/path/prefix';
    /* eslint-disable-next-line @typescript-eslint/no-explicit-any */
    (global as any).GLOBAL_CONSOLES_LINK = '';
    const app = mount(
      <GlobalVarsProvider>
        <Navigation />
      </GlobalVarsProvider>
    );
    expect(
      app.contains(
        <NavItem>
          <NavLink>Consoles</NavLink>
        </NavItem>
      )
    ).toBeFalsy();
  });
});
