import {contextRoot} from '../publicPath';
import {component} from 'flightjs';
import {errToStr} from '../../js/component_ui/error';
import $ from 'jquery';
import queryString from 'query-string';
import {traceSummary, traceSummariesToMustache} from '../component_ui/traceSummary';
import {SPAN_V1} from '../spanConverter';
import {correctForClockSkew} from '../skew';

export function convertToApiQuery(source) {
  const query = Object.assign({}, source);
  // zipkin's api looks back from endTs
  if (query.lookback !== 'custom') {
    delete query.startTs;
    delete query.endTs;
  }
  if (query.startTs) {
    if (query.endTs > query.startTs) {
      query.lookback = String(query.endTs - query.startTs);
    }
    delete query.startTs;
  }
  if (query.lookback === 'custom') {
    delete query.lookback;
  }
  if (query.serviceName === 'all') {
    delete query.serviceName;
  }
  if (query.spanName === 'all') {
    delete query.spanName;
  }
  // delete any parameters unused on the server
  Object.keys(query).forEach(key => {
    if (query[key] === '') {
      delete query[key];
    }
  });
  delete query.sortOrder;
  return query;
}

export default component(function DefaultData() {
  this.after('initialize', function() {
    const query = queryString.parse(window.location.search);
    if (!query.serviceName) {
      this.trigger('defaultPageModelView', {traces: []});
      return;
    }
    const apiQuery = convertToApiQuery(query);
    const apiURL = `${contextRoot}api/v2/traces?${queryString.stringify(apiQuery)}`;
    $.ajax(apiURL, {
      type: 'GET',
      dataType: 'json'
    }).done(traces => {
      const summaries = traces.map(raw => {
        const v1Trace = raw.map(SPAN_V1.convert);
        const mergedTrace = SPAN_V1.mergeById(v1Trace);
        const clockSkewCorrectedTrace = correctForClockSkew(mergedTrace);
        return traceSummary(clockSkewCorrectedTrace);
      });

      const modelview = {
        traces: traceSummariesToMustache(apiQuery.serviceName, summaries),
        apiURL,
        rawResponse: traces
      };
      this.trigger('defaultPageModelView', modelview);
    }).fail(e => {
      this.trigger('defaultPageModelView', {traces: [],
                                            queryError: errToStr(e)});
    });
  });
});
