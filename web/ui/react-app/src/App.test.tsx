import * as React from 'react';
import { shallow } from 'enzyme';
import App from './App';
import Navigation from './Navbar';
import { Container } from 'reactstrap';
import { Router } from '@reach/router';
import { Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList } from './pages';

describe('App', () => {
  const app = shallow(<App pathPrefix="/path/prefix" />);

  it('navigates', () => {
    expect(app.find(Navigation)).toHaveLength(1);
  });
  it('routes', () => {
    [Alerts, Config, Flags, Rules, ServiceDiscovery, Status, Targets, TSDBStatus, PanelList].forEach(component => {
      const c = app.find(component);
      expect(c).toHaveLength(1);
      expect(c.prop('pathPrefix')).toBe('/path/prefix');
    });
    expect(app.find(Router)).toHaveLength(1);
    expect(app.find(Container)).toHaveLength(1);
  });
});
