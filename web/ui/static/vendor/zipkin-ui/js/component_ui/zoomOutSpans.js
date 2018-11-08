import {component} from 'flightjs';

export default component(function zoomOutSpans() {
  this.zoomOut = function() {
    this.trigger('uiZoomOutSpans');
  };

  this.after('initialize', function() {
    this.on('click', this.zoomOut);
  });
});
