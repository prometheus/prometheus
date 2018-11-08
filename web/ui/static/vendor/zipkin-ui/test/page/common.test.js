import $ from 'jquery';
import CommonUI from '../../js/page/common';

describe('CommonUI', () => {
  it('should render the layout and create the .content container', () => {
    const container = $('<div />');
    CommonUI.attachTo(container);
    container.find('.content').length.should.equal(1);
  });
});
