import * as React from 'react';
import { shallow } from 'enzyme';
import Navigation from './Navbar';
import { NavItem, NavLink } from 'reactstrap';

describe('Navbar should contain console Link', () => {
  it('with non-empty consoleslink', () => {
    const app = shallow(<Navigation pathPrefix="/path/prefix" consolesLink="/path/consoles" />);
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
    const app = shallow(<Navigation pathPrefix="/path/prefix" consolesLink={null} />);
    expect(
      app.contains(
        <NavItem>
          <NavLink>Consoles</NavLink>
        </NavItem>
      )
    ).toBeFalsy();
  });
});
