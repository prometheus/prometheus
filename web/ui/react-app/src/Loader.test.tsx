import * as React from 'react';
import { shallow } from 'enzyme';
import Loader from './Loader';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

describe('Loader', () => {
  it('renders a spinning loader', () => {
    const loader = shallow(<Loader />);
    const icon = loader.find(FontAwesomeIcon);
    expect(icon).toHaveLength(1);
    expect(icon.prop('spin')).toBe(true);
    expect(icon.prop('icon')).toEqual(faSpinner);
  });
});
