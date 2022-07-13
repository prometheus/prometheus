import * as React from 'react';
import { shallow } from 'enzyme';
import App from './App';
import Navigation from './Navbar';
import { Container } from 'reactstrap';
import { Route } from 'react-router-dom';
import {
  AgentPage,
  AlertsPage,
  ConfigPage,
  FlagsPage,
  RulesPage,
  ServiceDiscoveryPage,
  StatusPage,
  TargetsPage,
  TSDBStatusPage,
  PanelListPage,
} from './pages';

describe('App', () => {
  const app = shallow(<App consolesLink={null} agentMode={false} />);

  it('navigates', () => {
    expect(app.find(Navigation)).toHaveLength(1);
  });
  it('routes', () => {
    [
      AgentPage,
      AlertsPage,
      ConfigPage,
      FlagsPage,
      RulesPage,
      ServiceDiscoveryPage,
      StatusPage,
      TargetsPage,
      TSDBStatusPage,
      PanelListPage,
    ].forEach((component) => {
      const c = app.find(component);
      expect(c).toHaveLength(1);
    });
    expect(app.find(Route)).toHaveLength(10);
    expect(app.find(Container)).toHaveLength(1);
  });
});
