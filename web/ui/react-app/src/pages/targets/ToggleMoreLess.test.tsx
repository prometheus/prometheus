import React from 'react';
import { shallow } from 'enzyme';
import { Button } from 'reactstrap';
import { ToggleMoreLess } from './ToggleMoreLess';

describe('ToggleMoreLess', () => {
  const showMoreValue = false;
  const defaultProps = {
    event: (): void => {
      tggleBtn.setProps({ showMore: !showMoreValue });
    },
    showMore: showMoreValue,
  };
  const tggleBtn = shallow(<ToggleMoreLess {...defaultProps} />);

  it('renders a show more btn at start', () => {
    const btn = tggleBtn.find(Button);
    expect(btn).toHaveLength(1);
    expect(btn.prop('color')).toEqual('primary');
    expect(btn.prop('size')).toEqual('xs');
    expect(btn.render().text()).toEqual('show more');
  });

  it('renders a show less btn if clicked', () => {
    tggleBtn.find(Button).simulate('click');
    expect(
      tggleBtn
        .find(Button)
        .render()
        .text()
    ).toEqual('show less');
  });
});
