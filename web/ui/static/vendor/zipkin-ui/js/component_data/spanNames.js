import {contextRoot} from '../publicPath';
import {component} from 'flightjs';
import {getError} from '../../js/component_ui/error';
import $ from 'jquery';

export default component(function spanNames() {
  this.updateSpanNames = function(ev, serviceName) {
    if (!serviceName) {
      this.trigger('dataSpanNames', {spans: []});
      return;
    }
    $.ajax(`${contextRoot}api/v2/spans?serviceName=${serviceName}`, {
      type: 'GET',
      dataType: 'json'
    }).done(spans => {
      this.trigger('dataSpanNames', {spans: spans.sort()});
    }).fail(e => {
      this.trigger('uiServerError', getError('cannot load span names', e));
    });
  };

  this.after('initialize', function() {
    this.on('uiChangeServiceName', this.updateSpanNames);
    this.on('uiFirstLoadSpanNames', this.updateSpanNames);
  });
});
