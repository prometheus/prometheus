import React from 'react';
import { shallow } from 'enzyme';
import { targetGroups } from './__testdata__/testdata';
import JobPanel from './JobPanel';
import JobHeader from './JobHeader';
import { Collapse } from 'reactstrap';
import JobDetails from './JobDetails';

describe('JobPanel', () => {
  const defaultProps = {
    scrapeJob: 'blackbox',
    targetGroup: targetGroups.blackbox,
  };
  const jobPanel = shallow(<JobPanel {...defaultProps} />);
  it('renders a container', () => {
    const div = jobPanel.find('div').filterWhere(elem => elem.hasClass('container'));
    expect(div).toHaveLength(1);
  });

  it('renders a JobHeader', () => {
    const header = jobPanel.find(JobHeader);
    expect(header).toHaveLength(1);
    expect(header.prop('scrapeJob')).toEqual(defaultProps.scrapeJob);
    expect(header.prop('expanded')).toBe(true);
    expect(header.prop('up')).toEqual(2);
    expect(header.prop('targetTotal')).toEqual(3);
  });

  it('renders a Collapse component', () => {
    const collapse = jobPanel.find(Collapse);
    expect(collapse.prop('isOpen')).toBe(true);
  });

  it('renders JobDetails', () => {
    const jobDetails = jobPanel.find(JobDetails);
    expect(jobDetails.prop('targetGroup')).toEqual(defaultProps.targetGroup);
  });
});
