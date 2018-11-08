// eslint-disable no-nested-ternary
import _ from 'lodash';
import moment from 'moment';

import {Constants} from './traceConstants';

function endpointsForSpan(span) {
  return _.union(
    (span.annotations || []).map(a => a.endpoint),
    (span.binaryAnnotations || []).map(a => a.endpoint)
  ).filter(h => h != null);
}

// What's the total duration of the spans in this trace?
export function traceDuration(spans) {
  function makeList({timestamp, duration}) {
    if (!timestamp) {
      return [];
    } else if (!duration) {
      return [timestamp];
    } else {
      return [timestamp, timestamp + duration];
    }
  }

  // turns (timestamp, timestamp + duration) into an ordered list
  const timestamps = _(spans).flatMap(makeList).sort().value();

  if (timestamps.length < 2) {
    return null;
  } else {
    const first = _.head(timestamps);
    const last = _.last(timestamps);
    return last - first;
  }
}

export function getServiceNames(span) {
  return _(endpointsForSpan(span))
      .map((ep) => ep.serviceName)
      .filter((name) => name != null && name !== '')
      .uniq().value();
}

function findServiceNameForBinaryAnnotation(span, key) {
  const binaryAnnotation = _(span.binaryAnnotations || []).find((ann) =>
            ann.key === key
            && ann.endpoint != null
            && ann.endpoint.serviceName != null
            && ann.endpoint.serviceName !== '');
  return binaryAnnotation ? binaryAnnotation.endpoint.serviceName : null;
}

function findServiceNameForAnnotation(span, values) {
  const annotation = _(span.annotations || []).find((ann) =>
            values.indexOf(ann.value) !== -1
            && ann.endpoint != null
            && ann.endpoint.serviceName != null
            && ann.endpoint.serviceName !== '');
  return annotation ? annotation.endpoint.serviceName : null;
}

export function getServiceName(span) {
  // Most authoritative is the label of the server's endpoint
  const serverAddressServiceName = findServiceNameForBinaryAnnotation(span, Constants.SERVER_ADDR);
  if (serverAddressServiceName) {
    return serverAddressServiceName;
  }

  // Next, the label of any server annotation, logged by an instrumented server
  const serverAnnotationServiceName = findServiceNameForAnnotation(span, Constants.CORE_SERVER);
  if (serverAnnotationServiceName) {
    return serverAnnotationServiceName;
  }

  // Next, the label of any messaging annotation, logged by an instrumented producer or consumer
  const messageAnnotationServiceName = findServiceNameForAnnotation(span, Constants.CORE_MESSAGE);
  if (messageAnnotationServiceName) {
    return messageAnnotationServiceName;
  }

  // Next is the label of the client's endpoint
  const clientAddressServiceName = findServiceNameForBinaryAnnotation(span, Constants.CLIENT_ADDR);
  if (clientAddressServiceName) {
    return clientAddressServiceName;
  }

  // Next is the label of any client annotation, logged by an instrumented client
  const clientAnnotationServiceName = findServiceNameForAnnotation(span, Constants.CORE_CLIENT);
  if (clientAnnotationServiceName) {
    return clientAnnotationServiceName;
  }

  // Next is the label of the broker's endpoint
  const brokerAddressServiceName = findServiceNameForBinaryAnnotation(span, Constants.MESSAGE_ADDR);
  if (brokerAddressServiceName) {
    return brokerAddressServiceName;
  }

  // Then is the label of the local component's endpoint
  const localServiceName = findServiceNameForBinaryAnnotation(span, Constants.LOCAL_COMPONENT);
  if (localServiceName) {
    return localServiceName;
  }

  // Finally, anything so that the service name isn't blank!
  const allServiceNames = getServiceNames(span);
  return allServiceNames.length === 0 ? null : allServiceNames[0];
}

function getSpanTimestamps(spans) {
  return _(spans).flatMap((span) => getServiceNames(span).map((serviceName) => ({
    name: serviceName,
    timestamp: span.timestamp,
    duration: span.duration
  }))).value();
}


// returns 'critical' if one of the spans has an ERROR binary annotation, else
// returns 'transient' if one of the spans has an ERROR annotation, else
// returns 'none'
export function getTraceErrorType(spans) {
  let traceType = 'none';
  for (let i = 0; i < spans.length; i++) {
    const span = spans[i];
    if (_(span.binaryAnnotations || []).findIndex(ann => ann.key === Constants.ERROR) !== -1) {
      return 'critical';
    } else if (traceType === 'none' &&
               _(span.annotations || []).findIndex(ann => ann.value === Constants.ERROR) !== -1) {
      traceType = 'transient';
    }
  }
  return traceType;
}

function endpointEquals(e1, e2) {
  return (e1.ipv4 === e2.ipv4 || e1.ipv6 === e2.ipv6)
    && e1.port === e2.port && e1.serviceName === e2.serviceName;
}

export function traceSummary(spans = []) {
  if (spans.length === 0 || !spans[0].timestamp) {
    return null;
  } else {
    const duration = traceDuration(spans) || 0;
    const endpoints = _(spans).flatMap(endpointsForSpan).uniqWith(endpointEquals).value();
    const traceId = spans[0].traceId;
    const timestamp = spans[0].timestamp;
    const spanTimestamps = getSpanTimestamps(spans);
    const errorType = getTraceErrorType(spans);
    const totalSpans = spans.length;
    return {
      traceId,
      timestamp,
      duration,
      spanTimestamps,
      endpoints,
      errorType,
      totalSpans
    };
  }
}

export function totalServiceTime(stamps, acc = 0) {
  // This is a recursive function that performs arithmetic on duration
  // If duration is undefined, it will infinitely recurse. Filter out that case
  const filtered = stamps.filter((s) => s.duration);
  if (filtered.length === 0) {
    return acc;
  } else {
    const ts = _(filtered).minBy((s) => s.timestamp);
    const [current, next] = _(filtered)
        .partition((t) =>
          t.timestamp >= ts.timestamp
          && t.timestamp + t.duration <= ts.timestamp + ts.duration)
        .value();
    const endTs = Math.max(...current.map((t) => t.timestamp + t.duration));
    return totalServiceTime(next, acc + (endTs - ts.timestamp));
  }
}

function formatDate(timestamp, utc) {
  let m = moment(timestamp / 1000);
  if (utc) {
    m = m.utc();
  }
  return m.format('MM-DD-YYYYTHH:mm:ss.SSSZZ');
}

export function getGroupedTimestamps(summary) {
  return _(summary.spanTimestamps).groupBy((sts) => sts.name).value();
}

export function getServiceDurations(groupedTimestamps) {
  return _(groupedTimestamps).toPairs().map(([name, sts]) => ({
    name,
    count: sts.length,
    max: parseInt(Math.max(...sts.map(t => t.duration)) / 1000, 10)
  })).sortBy('name').value();
}

export function mkDurationStr(duration) {
  if (duration === 0 || typeof duration === 'undefined') {
    return '';
  } else if (duration < 1000) {
    return `${duration}μ`;
  } else if (duration < 1000000) {
    return `${(duration / 1000).toFixed(3)}ms`;
  } else {
    return `${(duration / 1000000).toFixed(3)}s`;
  }
}

function removeEmptyFromArray(array) {
  const newArray = [];
  for (let i = 0; i < array.length; i++) {
    if (array[i]) {
      newArray.push(array[i]);
    }
  }
  return newArray;
}

export function traceSummariesToMustache(serviceName = null, traceSummaries, utc = false) {
  if (traceSummaries.length === 0) {
    return [];
  } else {
    const traceSummariesCleaned = removeEmptyFromArray(traceSummaries);
    const maxDuration = Math.max(...traceSummariesCleaned.map((s) => s.duration)) / 1000;

    return traceSummariesCleaned.map((t) => {
      const duration = t.duration / 1000;
      const groupedTimestamps = getGroupedTimestamps(t);
      const serviceDurations = getServiceDurations(groupedTimestamps);

      let serviceTime;
      if (!serviceName || !groupedTimestamps[serviceName]) {
        serviceTime = 0;
      } else {
        serviceTime = totalServiceTime(groupedTimestamps[serviceName]);
      }

      const startTs = formatDate(t.timestamp, utc);
      const durationStr = mkDurationStr(t.duration);
      const servicePercentage = parseInt(
          parseFloat(serviceTime) / parseFloat(t.duration) * 100,
        10);
      const width = parseInt(parseFloat(duration) / parseFloat(maxDuration) * 100, 10);
      const infoClass = t.errorType === 'none' ? '' : `trace-error-${t.errorType}`;

      return {
        traceId: t.traceId,
        startTs,
        timestamp: t.timestamp,
        duration,
        durationStr,
        servicePercentage,
        totalSpans: t.totalSpans,
        serviceDurations,
        width,
        infoClass
      };
    }).sort((t1, t2) => {
      const durationComparison = t2.duration - t1.duration;
      if (durationComparison === 0) {
        return t1.traceId.localeCompare(t2.traceId);
      } else {
        return durationComparison;
      }
    });
  }
}
