import {component} from 'flightjs';
import $ from 'jquery';
import chosen from 'chosen-npm/public/chosen.jquery.js'; // eslint-disable-line no-unused-vars
import queryString from 'query-string';

export default component(function serviceName() {
  this.onChange = function() {
    this.triggerChange(this.$node.val());
  };

  this.triggerChange = function(name) {
    this.$node.trigger('uiChangeServiceName', name);
  };

  this.updateServiceNameDropdown = function(ev, data) {
    $('#serviceName').empty();
    this.$node.append($($.parseHTML('<option value="all">all</option>')));

    $.each(data.names, (i, item) => {
      $('<option>').val(item).text(item).appendTo('#serviceName');
    });

    this.$node.find(`[value="${data.lastServiceName}"]`).attr('selected', 'selected');

    this.trigger('chosen:updated');

    // On the first view there won't be a selected or "last" service
    // name.  Instead the first service at the top of the list will be
    // displayed, so load the span names for the top service too.
    if (!data.lastServiceName && data.names && data.names.length > 1) {
      this.$node.trigger('uiFirstLoadSpanNames', data.names[0]);
    }
  };

  this.after('initialize', function() {
    const name = queryString.parse(window.location.search).serviceName;
    this.triggerChange(name);

    this.$node.chosen({search_contains: true});
    this.$node.next('.chosen-container').css('width', '100%');

    this.on('change', this.onChange);
    this.on(document, 'dataServiceNames', this.updateServiceNameDropdown);
  });
});
