import * as React from 'react';
import { shallow } from 'enzyme';
import Navigation from './Navbar';
import { NavItem, NavLink } from 'reactstrap';

describe('Navbar should contains console Link', () => {
  it('with non-empty consoleslinks', () => {
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

describe('Navbar should not contains consoles link', () => {
  it('with empty string in consolesLinks ', () => {
    const app = shallow(<Navigation pathPrefix="/path/prefix" consolesLink="" />);
    expect(
      app.contains(
        <NavItem>
          <NavLink>Consoles</NavLink>
        </NavItem>
      )
    ).toBeFalsy();
  });
});
