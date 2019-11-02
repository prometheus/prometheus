import React from 'react';
import { shallow } from 'enzyme';
import JobHeader from './JobHeader';
import sinon from 'sinon';
import { Button } from 'reactstrap';

describe('Job Header', () => {
  const defaultProps = {
    expanded: true,
    scrapeJob: 'prometheus',
    setExpanded: sinon.spy(),
    targetTotal: 1,
    up: 1,
  };
  const jobHeader = shallow(<JobHeader {...defaultProps} />);

  it('renders an h2', () => {
    expect(jobHeader.find('h2')).toHaveLength(1);
  });

  it('renders an anchor with up count', () => {
    const anchor = jobHeader.find('a');
    expect(anchor).toHaveLength(1);
    expect(anchor.prop('id')).toEqual('job-prometheus');
    expect(anchor.prop('href')).toEqual('#job-prometheus');
    expect(anchor.text()).toEqual('prometheus (1/1 up)');
    expect(anchor.prop('className')).toEqual('normal');
  });

  it('renders the anchor in red if up count is less than target total', () => {
    const props = {
      ...defaultProps,
      scrapeJob: 'blackbox',
      targetTotal: 4,
      up: 3,
    };
    const jobHeader = shallow(<JobHeader {...props} />);
    const anchor = jobHeader.find('a');
    expect(anchor.text()).toEqual('blackbox (3/4 up)');
    expect(anchor.prop('className')).toEqual('danger');
  });

  it('renders a show less btn if expanded', () => {
    const btn = jobHeader.find(Button);
    expect(btn).toHaveLength(1);
    expect(btn.prop('className')).toEqual('expand-btn');
    expect(btn.prop('color')).toEqual('primary');
    expect(btn.prop('size')).toEqual('xs');
    expect(btn.render().text()).toEqual('show less');
  });

  it('renders a show more btn if collapsed', () => {
    const props = {
      ...defaultProps,
      expanded: false,
    };
    const jobHeader = shallow(<JobHeader {...props} />);
    const btn = jobHeader.find(Button);
    expect(btn.render().text()).toEqual('show more');
  });

  it('calls setExpanded when the btn is clicked', () => {
    const btn = jobHeader.find(Button);
    btn.simulate('click');
    expect(defaultProps.setExpanded.getCalls()[0].args[0]).toEqual(!defaultProps.expanded);
  });
});
