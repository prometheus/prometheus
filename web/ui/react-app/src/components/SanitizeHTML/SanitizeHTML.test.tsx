import * as React from 'react';
import { shallow } from 'enzyme';
import SanitizeHTML from '.';

describe('SanitizeHTML', () => {
  it(`renders allowed html`, () => {
    const props = {
      allowedTags: ['strong'],
    };
    const html = shallow(<SanitizeHTML {...props}>{'<strong>text</strong>'}</SanitizeHTML>);
    const elem = html.find('div');
    expect(elem).toHaveLength(1);
    expect(elem.html()).toEqual(`<div><strong>text</strong></div>`);
  });

  it('does not render disallowed tags', () => {
    const props = {
      tag: 'span' as keyof JSX.IntrinsicElements,
      allowedTags: ['strong'],
    };
    const html = shallow(<SanitizeHTML {...props}>{'<a href="www.example.com">link</a>'}</SanitizeHTML>);
    const elem = html.find('span');
    expect(elem.html()).toEqual('<span>link</span>');
  });
});
