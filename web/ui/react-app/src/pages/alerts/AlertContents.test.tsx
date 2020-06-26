import React from 'react';
import { shallow } from 'enzyme';
import AlertsContent from './AlertContents';

describe('AlertsContent', () => {
  const defaultProps = {
    groups: [],
    statsCount: {
      inactive: 0,
      pending: 0,
      firing: 0,
    },
  };
  const wrapper = shallow(<AlertsContent {...defaultProps} />);

  it('matches a snapshot', () => {
    expect(wrapper).toMatchSnapshot();
  });

  [
    { selector: '#inactive-toggler', propName: 'inactive' },
    { selector: '#pending-toggler', propName: 'pending' },
    { selector: '#firing-toggler', propName: 'firing' },
  ].forEach(testCase => {
    it(`toggles the ${testCase.propName} checkbox from true to false when clicked and back to true when clicked again`, () => {
      wrapper.find(testCase.selector).invoke('onClick')(testCase.propName);
      expect(wrapper.find(testCase.selector).prop('checked')).toBe(false);
      wrapper.find(testCase.selector).invoke('onClick')(testCase.propName);
      expect(wrapper.find(testCase.selector).prop('checked')).toBe(true);
    });
  });

  it('toggles the "annotations" checkbox from false to true when clicked and back to false when clicked again', () => {
    wrapper.find('#show-annotations-toggler').invoke('onClick')();
    expect(wrapper.find('#show-annotations-toggler').prop('checked')).toBe(true);
    wrapper.find('#show-annotations-toggler').invoke('onClick')();
    expect(wrapper.find('#show-annotations-toggler').prop('checked')).toBe(false);
  });
});
