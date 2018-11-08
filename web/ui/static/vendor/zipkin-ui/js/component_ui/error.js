import {component} from 'flightjs';
import $ from 'jquery';

export default component(function ErrorUI() {
  this.after('initialize', function() {
    this.on(document, 'uiServerError', function(evt, e) {
      this.$node.append($('<div></div>').text(`ERROR: ${e.desc}: ${e.message}`));
      this.$node.show();
    });
  });
});

// converts an jqXhr error to a string
export function errToStr(e) {
  return e.responseJSON ? e.responseJSON.message : `${e.responseText}`;
}

// transforms an ajax error into something that is passed to
// trigger('uiServerError')
export function getError(desc, e) {
  return {
    desc,
    message: errToStr(e)
  };
}
