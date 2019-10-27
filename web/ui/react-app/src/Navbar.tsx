import React, { useState } from 'react';
import { Link } from "@reach/router";
import {
  Collapse,
  Navbar,
  NavbarToggler,
  Nav,
  NavItem,
  NavLink,
  UncontrolledDropdown,
  DropdownToggle,
  DropdownMenu,
  DropdownItem,
} from 'reactstrap';

const Navigation = () => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  return (
    <Navbar className="mb-3" dark color="dark" expand="md">  
      <NavbarToggler onClick={toggle}/>
      <Link className="pt-0 navbar-brand" to="/react/graph">Prometheus</Link>
      <Collapse isOpen={isOpen} navbar style={{ justifyContent: 'space-between' }}>
        <Nav className="ml-0" navbar>
          <NavItem>
            <NavLink tag={Link} to="/react/alerts">Alerts</NavLink>
          </NavItem>
          <NavItem>
            <NavLink tag={Link} to="/react/graph">Graph</NavLink>
          </NavItem>
          <UncontrolledDropdown nav inNavbar>
            <DropdownToggle nav caret>Status</DropdownToggle>
            <DropdownMenu>
              <DropdownItem tag={Link} to="/react/status">Runtime & Build Information</DropdownItem>
              <DropdownItem tag={Link} to="/react/flags">Command-Line Flags</DropdownItem>
              <DropdownItem tag={Link} to="/react/config">Configuration</DropdownItem>
              <DropdownItem tag={Link} to="/react/rules">Rules</DropdownItem>
              <DropdownItem tag={Link} to="/react/targets">Targets</DropdownItem>
              <DropdownItem tag={Link} to="/react/service-discovery">Service Discovery</DropdownItem>
            </DropdownMenu>
          </UncontrolledDropdown>
          <NavItem>
            <NavLink href="https://prometheus.io/docs/prometheus/latest/getting_started/">Help</NavLink>
          </NavItem>
          <NavItem>
            <NavLink tag={Link} to="../../graph">Classic UI</NavLink>
          </NavItem>
        </Nav>
      </Collapse>
    </Navbar>
  );
}

export default Navigation;
