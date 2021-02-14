import React, { Component } from 'react';
import { shallow, ShallowWrapper } from 'enzyme';
import { Button, ButtonGroup } from 'reactstrap';
import Filter, { FilterData, FilterProps } from './Filter';
import sinon, { SinonSpy } from 'sinon';

describe('Filter', () => {
  const initialExpanded = {
    scrapePool1: true,
    scrapePool2: true,
  };
  const initialState: FilterData = {
    showHealthy: true,
    showUnhealthy: true,
    expanded: initialExpanded,
  };
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

  it('renders an expansion filter button that is inactive', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('expansion'));
    expect(btn.prop('active')).toBe(false);
    expect(btn.prop('color')).toBe('primary');
  });

  it('renders an all filter button which shows all targets', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('all'));
    btn.simulate('click');
    expect(setFilter.calledOnce).toBe(true);
    expect(setFilter.getCall(0).args[0]).toEqual({ showHealthy: true, showUnhealthy: true, expanded: initialExpanded });
  });

  it('renders an unhealthy filter button which filters targets', () => {
    const btn = filterWrapper.find(Button).filterWhere((btn): boolean => btn.hasClass('unhealthy'));
    btn.simulate('click');
    expect(setFilter.calledOnce).toBe(true);
    expect(setFilter.getCall(0).args[0]).toEqual({ showHealthy: false, showUnhealthy: true, expanded: initialExpanded });
  });

  describe('Expansion filter', () => {
    [
      {
        name: 'expanded => collapsed',
        initial: initialExpanded,
        final: { scrapePool1: false, scrapePool2: false },
        text: 'Collapse All',
      },
      {
        name: 'collapsed => expanded',
        initial: { scrapePool1: false, scrapePool2: false },
        final: initialExpanded,
        text: 'Expand All',
      },
      {
        name: 'some expanded => expanded',
        initial: { scrapePool1: true, scrapePool2: false },
        final: initialExpanded,
        text: 'Expand All',
      },
    ].forEach(({ name, text, initial, final }) => {
      it(`filters targets ${name}`, (): void => {
        const filter = { ...initialState, expanded: initial };
        const callback = sinon.spy();
        const filterW = shallow(<Filter filter={filter} setFilter={callback} />);
        const btn = filterW.find(Button).filterWhere((btn): boolean => btn.hasClass('expansion'));
        expect(btn.children().text()).toEqual(text);
        btn.simulate('click');
        expect(callback.calledOnce).toBe(true);
        expect(callback.getCall(0).args[0]).toEqual({
          ...initialState,
          expanded: final,
        });
      });
    });
  });
});
