import {component} from 'flightjs';
import $ from 'jquery';
import TraceData from '../component_data/trace';
import FullPageSpinnerUI from '../component_ui/fullPageSpinner';
import SpanPanelUI from '../component_ui/spanPanel';
import TraceUI from '../component_ui/trace';
import FilterLabelUI from '../component_ui/filterLabel';
import ZoomOut from '../component_ui/zoomOutSpans';
import {traceTemplate} from '../templates';
import {contextRoot} from '../publicPath';

const TracePageComponent = component(function TracePage() {
  this.after('initialize', function() {
    TraceData.attachTo(document, {
      traceId: this.attr.traceId,
      logsUrl: this.attr.config('logsUrl')
    });
    this.on(document, 'tracePageModelView', function(ev, data) {
      this.$node.html(traceTemplate({
        contextRoot,
        ...data.modelview
      }));

      FullPageSpinnerUI.attachTo('#fullPageSpinner');
      SpanPanelUI.attachTo('#spanPanel');
      TraceUI.attachTo('#trace-container');
      FilterLabelUI.attachTo('.service-filter-label');
      ZoomOut.attachTo('#zoomOutSpans');
      $('.annotation:not(.core)').tooltip({placement: 'left'});
    });
  });
});

export default function initializeTrace(traceId, config) {
  TracePageComponent.attachTo('.content', {
    traceId,
    config
  });
}
