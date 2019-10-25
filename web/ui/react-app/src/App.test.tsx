import * as React from 'react';
import { shallow } from 'enzyme';
import App from './App';
import Navigation from './Navbar';
import { Container } from 'reactstrap';
import { Router } from '@reach/router';
import { Alerts, Config, Flags, Rules, Services, Status, Targets, PanelList } from './pages';

describe('App', () => {
  const app = shallow(<App />);

  it('navigates', () => {
    expect(app.find(Navigation)).toHaveLength(1);
  });
  it('routes', () => {
    [Alerts, Config, Flags, Rules, Services, Status, Targets, PanelList].forEach(component =>
      expect(app.find(component)).toHaveLength(1)
    );
    expect(app.find(Router)).toHaveLength(1);
    expect(app.find(Container)).toHaveLength(1);
  });
});
