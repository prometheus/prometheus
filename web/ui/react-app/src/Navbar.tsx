import React, { FC, useState } from 'react';
import { Link } from '@reach/router';
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
import { usePathPrefix } from './contexts/PathPrefixContext';

interface NavbarProps {
  consolesLink: string | null;
}

const Navigation: FC<NavbarProps> = ({ consolesLink }) => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  const pathPrefix = usePathPrefix();
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} />
      <Link className="pt-0 navbar-brand" to={`${pathPrefix}/graph`}>
        Prometheus
      </Link>
      <Collapse isOpen={isOpen} navbar style={{ justifyContent: 'space-between' }}>
        <Nav className="ml-0" navbar>
          {consolesLink !== null && (
            <NavItem>
              <NavLink href={consolesLink}>Consoles</NavLink>
            </NavItem>
          )}
          <NavItem>
            <NavLink tag={Link} to={`${pathPrefix}/alerts`}>
              Alerts
            </NavLink>
          </NavItem>
          <NavItem>
            <NavLink tag={Link} to={`${pathPrefix}/graph`}>
              Graph
            </NavLink>
          </NavItem>
          <UncontrolledDropdown nav inNavbar>
            <DropdownToggle nav caret>
              Status
            </DropdownToggle>
            <DropdownMenu>
              <DropdownItem tag={Link} to={`${pathPrefix}/status`}>
                Runtime & Build Information
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/tsdb-status`}>
                TSDB Status
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/flags`}>
                Command-Line Flags
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/config`}>
                Configuration
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/rules`}>
                Rules
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/targets`}>
                Targets
              </DropdownItem>
              <DropdownItem tag={Link} to={`${pathPrefix}/service-discovery`}>
                Service Discovery
              </DropdownItem>
            </DropdownMenu>
          </UncontrolledDropdown>
          <NavItem>
            <NavLink href="https://prometheus.io/docs/prometheus/latest/getting_started/">Help</NavLink>
          </NavItem>
          <NavItem>
            <NavLink href={`${pathPrefix}/classic/graph${window.location.search}`}>Classic UI</NavLink>
          </NavItem>
        </Nav>
      </Collapse>
    </Navbar>
  );
};

export default Navigation;
