import {component} from 'flightjs';
import $ from 'jquery';
import timeago from 'timeago'; // eslint-disable-line no-unused-vars
import queryString from 'query-string';
import DefaultData from '../component_data/default';
import SpanNamesData from '../component_data/spanNames';
import ServiceNamesData from '../component_data/serviceNames';
import ServiceNameUI from '../component_ui/serviceName';
import SpanNameUI from '../component_ui/spanName';
import LookbackUI from '../component_ui/lookback';
import InfoPanelUI from '../component_ui/infoPanel';
import InfoButtonUI from '../component_ui/infoButton';
import TraceFiltersUI from '../component_ui/traceFilters';
import TracesUI from '../component_ui/traces';
import TimeStampUI from '../component_ui/timeStamp';
import BackToTop from '../component_ui/backToTop';
import {defaultTemplate} from '../templates';
import {contextRoot} from '../publicPath';

const DefaultPageComponent = component(function DefaultPage() {
  const sortOptions = [
    {value: 'service-percentage-desc', text: 'Service Percentage: Longest First'},
    {value: 'service-percentage-asc', text: 'Service Percentage: Shortest First'},
    {value: 'duration-desc', text: 'Longest First'},
    {value: 'duration-asc', text: 'Shortest First'},
    {value: 'timestamp-desc', text: 'Newest First'},
    {value: 'timestamp-asc', text: 'Oldest First'}
  ];

  const sortSelected = function getSelector(selectedSortValue) {
    return function selected() {
      if (this.value === selectedSortValue) {
        return 'selected';
      }
      return '';
    };
  };

  this.after('initialize', function() {
    this.trigger(document, 'navigate', {route: 'zipkin/index'});

    const query = queryString.parse(window.location.search);

    this.on(document, 'defaultPageModelView', function(ev, modelView) {
      const limit = query.limit || this.attr.config('queryLimit');
      const minDuration = query.minDuration;
      const maxDuration = query.maxDuration;
      const endTs = query.endTs || new Date().getTime();
      const startTs = query.startTs || (endTs - this.attr.config('defaultLookback'));
      const serviceName = query.serviceName || '';
      const annotationQuery = query.annotationQuery || '';
      const sortOrder = query.sortOrder || 'duration-desc';
      const queryWasPerformed = serviceName && serviceName.length > 0;
      this.$node.html(defaultTemplate({
        limit,
        minDuration,
        maxDuration,
        startTs,
        endTs,
        serviceName,
        annotationQuery,
        queryWasPerformed,
        contextRoot,
        count: modelView.traces.length,
        sortOrderOptions: sortOptions,
        sortOrderSelected: sortSelected(sortOrder),
        apiURL: modelView.apiURL,
        ...modelView
      }));

      SpanNamesData.attachTo(document);
      ServiceNamesData.attachTo(document);
      ServiceNameUI.attachTo('#serviceName');
      SpanNameUI.attachTo('#spanName');
      LookbackUI.attachTo('#lookback');
      InfoPanelUI.attachTo('#infoPanel');
      InfoButtonUI.attachTo('button.info-request');
      TraceFiltersUI.attachTo('#trace-filters');
      TracesUI.attachTo('#traces');
      TimeStampUI.attachTo('#end-ts');
      TimeStampUI.attachTo('#start-ts');
      BackToTop.attachTo('#backToTop');

      $('.timeago').timeago();
      // Need to initialize the datepicker when the UI refershes. Can be optimized
      this.$date = this.$node.find('.date-input');
      this.$date.datepicker({format: 'yyyy-mm-dd'});
    });

    DefaultData.attachTo(document);
  });
});

export default function initializeDefault(config) {
  DefaultPageComponent.attachTo('.content', {config});
}
