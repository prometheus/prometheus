import React, { Component } from 'react';
import { shallow, ShallowWrapper } from 'enzyme';
import { Button, ButtonGroup } from 'reactstrap';
import Filter, { FilterData, FilterProps } from './Filter';
import sinon, { SinonSpy } from 'sinon';

describe('Filter', () => {
  const initialState: FilterData = { showHealthy: true, showUnhealthy: true };
  let setFilter: SinonSpy;
  let filterWrapper: ShallowWrapper<FilterProps, Readonly<{}>, Component<{}, {}, Component>>;
  beforeEach(() => {
    setFilter = sinon.spy();
    filterWrapper = shallow(<Filter filter={initialState} setFilter={setFilter} />);
  });

  it('renders a button group', () => {
    expect(filterWrapper.find(ButtonGroup)).toHaveLength(1);
  });

  it('renders an all filter button that is active by default', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('all'));
    expect(btn.prop('active')).toBe(true);
    expect(btn.prop('color')).toBe('primary');
  });

  it('renders an unhealthy filter button that is inactive by default', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('unhealthy'));
    expect(btn.prop('active')).toBe(false);
    expect(btn.prop('color')).toBe('primary');
  });

  it('renders an all filter button which shows all targets', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('all'));
    btn.simulate('click');
    expect(setFilter.calledOnce).toBe(true);
    expect(setFilter.getCall(0).args[0]).toEqual({ showHealthy: true, showUnhealthy: true });
  });

  it('renders an unhealthy filter button which filters targets', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('unhealthy'));
    btn.simulate('click');
    expect(setFilter.calledOnce).toBe(true);
    expect(setFilter.getCall(0).args[0]).toEqual({ showHealthy: false, showUnhealthy: true });
  });
});
