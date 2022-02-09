import React, { FC, useState } from 'react';
import { Link } from 'react-router-dom';
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
import { ThemeToggle } from './Theme';
import logo from './images/prometheus_logo_grey.svg';

interface NavbarProps {
  consolesLink: string | null;
  agentMode: boolean;
}

const Navigation: FC<NavbarProps> = ({ consolesLink, agentMode }) => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} className="mr-2" />
      <Link className="pt-0 navbar-brand" to={agentMode ? '/agent' : '/graph'}>
        <img src={logo} className="d-inline-block align-top" alt="Prometheus logo" title="Prometheus" />
        Prometheus{agentMode && ' Agent'}
      </Link>
      <Collapse isOpen={isOpen} navbar style={{ justifyContent: 'space-between' }}>
        <Nav className="ml-0" navbar>
          {consolesLink !== null && (
            <NavItem>
              <NavLink href={consolesLink}>Consoles</NavLink>
            </NavItem>
          )}
          {!agentMode && (
            <>
              <NavItem>
                <NavLink tag={Link} to="/alerts">
                  Alerts
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink tag={Link} to="/graph">
                  Graph
                </NavLink>
              </NavItem>
            </>
          )}
          <UncontrolledDropdown nav inNavbar>
            <DropdownToggle nav caret>
              Status
            </DropdownToggle>
            <DropdownMenu>
              <DropdownItem tag={Link} to="/status">
                Runtime & Build Information
              </DropdownItem>
              {!agentMode && (
                <DropdownItem tag={Link} to="/tsdb-status">
                  TSDB Status
                </DropdownItem>
              )}
              <DropdownItem tag={Link} to="/flags">
                Command-Line Flags
              </DropdownItem>
              <DropdownItem tag={Link} to="/config">
                Configuration
              </DropdownItem>
              {!agentMode && (
                <DropdownItem tag={Link} to="/rules">
                  Rules
                </DropdownItem>
              )}
              <DropdownItem tag={Link} to="/targets">
                Targets
              </DropdownItem>
              <DropdownItem tag={Link} to="/service-discovery">
                Service Discovery
              </DropdownItem>
            </DropdownMenu>
          </UncontrolledDropdown>
          <NavItem>
            <NavLink href="https://prometheus.io/docs/prometheus/latest/getting_started/">Help</NavLink>
          </NavItem>
        </Nav>
      </Collapse>
      <ThemeToggle />
    </Navbar>
  );
};

export default Navigation;
