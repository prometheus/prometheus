import * as React from 'react';
import { mount, shallow } from 'enzyme';
import PanelList from './pages/PanelList';
import Checkbox from './Checkbox';
import { Alert, Button } from 'reactstrap';
import Panel from './Panel';

describe('PanelList', () => {
  it('renders a query history checkbox', () => {
    const panelList = shallow(<PanelList />);
    const checkbox = panelList.find(Checkbox);
    expect(checkbox.prop('id')).toEqual('query-history-checkbox');
    expect(checkbox.prop('wrapperStyles')).toEqual({
      margin: '0 0 0 15px',
      alignSelf: 'center',
    });
    expect(checkbox.prop('defaultChecked')).toBe(false);
    expect(checkbox.children().text()).toBe('Enable query history');
  });

  it('renders an alert when no data is queried yet', () => {
    const panelList = mount(<PanelList />);
    const alert = panelList.find(Alert);
    expect(alert.prop('color')).toEqual('light');
    expect(alert.children().text()).toEqual('No data queried yet');
  });

  it('renders panels', () => {
    const panelList = shallow(<PanelList />);
    const panels = panelList.find(Panel);
    expect(panels.length).toBeGreaterThan(0);
  });

  it('renders a button to add a panel', () => {
    const panelList = shallow(<PanelList />);
    const btn = panelList.find(Button).filterWhere(btn => btn.prop('className') === 'add-panel-btn');
    expect(btn.prop('color')).toEqual('primary');
    expect(btn.children().text()).toEqual('Add Panel');
  });
});
