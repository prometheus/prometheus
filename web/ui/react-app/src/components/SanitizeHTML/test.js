/**
 * SanitizeHTML tests
 */
import React from 'react';
import ReactDOM from 'react-dom';
import SanitizeHTML from '../SanitizeHTML';

it('renders without crashing', () => {
  const div = document.createElement('div');
  ReactDOM.render(<SanitizeHTML />, div);
  ReactDOM.unmountComponentAtNode(div);
});
