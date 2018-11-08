import {component} from 'flightjs';

export default component(function infoButton() {
  this.requestInfoPanel = function() {
    this.trigger('uiRequestInfoPanel');
  };

  this.after('initialize', function() {
    this.on('click', this.requestInfoPanel);
  });
});
