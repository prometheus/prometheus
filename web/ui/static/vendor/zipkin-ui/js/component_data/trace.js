import {contextRoot} from '../publicPath';
import {component} from 'flightjs';
import $ from 'jquery';
import {getError} from '../../js/component_ui/error';
import traceToMustache from '../../js/component_ui/traceToMustache';
import {SPAN_V1} from '../spanConverter';
import {correctForClockSkew} from '../skew';

export function toContextualLogsUrl(logsUrl, traceId) {
  if (logsUrl) {
    return logsUrl.replace('{traceId}', traceId);
  }
  return logsUrl;
}

export default component(function TraceData() {
  this.after('initialize', function() {
    const traceId = this.attr.traceId;
    const logsUrl = toContextualLogsUrl(this.attr.logsUrl, traceId);
    $.ajax(`${contextRoot}api/v2/trace?id=${traceId}`, {
      type: 'GET',
      dataType: 'json'
    }).done(raw => {
      const v1Trace = raw.map(SPAN_V1.convert);
      const mergedTrace = SPAN_V1.mergeById(v1Trace);
      const clockSkewCorrectedTrace = correctForClockSkew(mergedTrace);
      const modelview = traceToMustache(clockSkewCorrectedTrace, logsUrl);
      this.trigger('tracePageModelView', {modelview, trace: raw});
    }).fail(e => {
      this.trigger('uiServerError',
                   getError(`Cannot load trace ${this.attr.traceId}`, e));
    });
  });
});
