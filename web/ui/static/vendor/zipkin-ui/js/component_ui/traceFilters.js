import {component} from 'flightjs';
import $ from 'jquery';

export default component(function traceFilters() {
  this.idFromService = function(service) {
    return `service-filter-${service.replace(/[^a-z0-9\-_]/gi, '-')}`;
  };

  this.addToFilter = function(ev, data) {
    if (this.$node.find(`[data-service-name='${data.value}']`).length) return;

    // TODO: should this be mustache instead?
    const $remove =
      $('<span>')
        .attr('class', 'badge service-filter-remove')
        .text('x')
        .on('click', function() { $(this).trigger('uiRemoveServiceNameFilter', data); });

    const $html =
      $('<span>')
        .attr('class', 'label service-filter-label service-tag-filtered')
        .attr('id', this.idFromService(data.value))
        .attr('data-serviceName', data.value)
        .text(data.value)
        .append($remove);

    this.$node.find('.service-tags').append($html);
  };

  this.removeFromFilter = function(ev, data) {
    this.$node.find(`#${this.idFromService(data.value)}`).remove();
  };

  this.updateTraces = function(ev, data) {
    this.$node.find('.filter-current').text(data.traces.length);
  };

  this.updateSortOrder = function(ev) {
    this.trigger(document, 'uiUpdateTraceSortOrder', {order: $(ev.target).val()});
  };

  this.after('initialize', function() {
    this.on(document, 'uiAddServiceNameFilter', this.addToFilter);
    this.on(document, 'uiRemoveServiceNameFilter', this.removeFromFilter);
    this.on(document, 'uiUpdateTraces', this.updateTraces);
    this.on('.sort-order', 'change', this.updateSortOrder);
  });
});
