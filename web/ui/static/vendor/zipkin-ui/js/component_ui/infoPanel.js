import {component} from 'flightjs';

import bootstrap // eslint-disable-line no-unused-vars
    from 'bootstrap-sass/assets/javascripts/bootstrap.js';

export default component(function infoPanel() {
  this.show = function() {
    this.$node.modal('show');
  };

  this.after('initialize', function() {
    this.$node.modal('hide');
    this.on(document, 'uiRequestInfoPanel', this.show);
  });
});
