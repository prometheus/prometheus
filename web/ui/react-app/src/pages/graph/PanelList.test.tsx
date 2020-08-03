import * as React from 'react';
import { shallow } from 'enzyme';
import PanelList, { PanelListContent } from './PanelList';
import Checkbox from '../../components/Checkbox';
import { Button } from 'reactstrap';
import Panel from './Panel';

describe('PanelList', () => {
  it('renders query history and local time checkboxes', () => {
    [
      { id: 'query-history-checkbox', label: 'Enable query history' },
      { id: 'use-local-time-checkbox', label: 'Use local time' },
    ].forEach((cb, idx) => {
      const panelList = shallow(<PanelList />);
      const checkbox = panelList.find(Checkbox).at(idx);
      expect(checkbox.prop('id')).toEqual(cb.id);
      expect(checkbox.prop('defaultChecked')).toBe(false);
      expect(checkbox.children().text()).toBe(cb.label);
    });
  });

  it('renders panels', () => {
    const panelList = shallow(<PanelListContent {...({ panels: [{ id: 'foo' }] } as any)} />);
    const panels = panelList.find(Panel);
    expect(panels.length).toBeGreaterThan(0);
  });

  it('renders a button to add a panel', () => {
    const panelList = shallow(<PanelListContent {...({ panels: [] } as any)} />);
    const btn = panelList.find(Button);
    expect(btn.prop('color')).toEqual('primary');
    expect(btn.children().text()).toEqual('Add Panel');
  });
});
