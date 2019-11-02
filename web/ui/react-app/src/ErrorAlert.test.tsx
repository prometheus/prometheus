import * as React from 'react';
import { shallow } from 'enzyme';
import Error from './ErrorAlert';
import { Alert } from 'reactstrap';

describe('Error', () => {
  it('renders a danger alert with a summary and a message', () => {
    const error = shallow(<Error summary="My summary" message="My message" />);
    const alert = error.find(Alert);
    expect(alert).toHaveLength(1);
    expect(alert.render().text()).toEqual('Error: My summary: My message');
  });
});
