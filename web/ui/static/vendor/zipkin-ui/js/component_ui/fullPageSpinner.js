import {component} from 'flightjs';

export default component(function fullPageSpinner() {
  this.requests = 0;

  this.showSpinner = function() {
    this.requests += 1;
    this.$node.show();
  };

  this.hideSpinner = function() {
    this.requests -= 1;
    if (this.requests === 0) {
      this.$node.hide();
    }
  };

  this.after('initialize', function() {
    this.on(document, 'uiShowFullPageSpinner', this.showSpinner);
    this.on(document, 'uiHideFullPageSpinner', this.hideSpinner);
  });
});
