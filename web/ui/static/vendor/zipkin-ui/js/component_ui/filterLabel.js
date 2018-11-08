import {component} from 'flightjs';

export default component(function filterLabel() {
  this.serviceName = '';

  this.toggleFilter = function() {
    const evt = this.$node.is('.service-tag-filtered') ?
      'uiRemoveServiceNameFilter' :
      'uiAddServiceNameFilter';
    this.trigger(evt, {value: this.serviceName});
  };

  this.filterAdded = function(e, data) {
    if (data.value === this.serviceName) {
      this.$node.addClass('service-tag-filtered');
    }
  };

  this.filterRemoved = function(e, data) {
    if (data.value === this.serviceName) {
      this.$node.removeClass('service-tag-filtered');
    }
  };

  this.after('initialize', function() {
    this.serviceName = this.$node.data('serviceName');
    this.on('click', this.toggleFilter);
    this.on(document, 'uiAddServiceNameFilter', this.filterAdded);
    this.on(document, 'uiRemoveServiceNameFilter', this.filterRemoved);
  });
});
