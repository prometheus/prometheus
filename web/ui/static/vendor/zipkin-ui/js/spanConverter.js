function toV1Endpoint(endpoint) {
  if (endpoint === undefined) {
    return undefined;
  }
  const res = {
    serviceName: endpoint.serviceName || '', // undefined is not allowed in v1
  };
  if (endpoint.ipv4) {
    res.ipv4 = endpoint.ipv4;
  }
  if (endpoint.ipv6) {
    res.ipv6 = endpoint.ipv6;
  }
  if (endpoint.port) {
    res.port = endpoint.port;
  }
  return res;
}

function toV1Annotation(ann, endpoint) {
  const res = {
    value: ann.value,
    timestamp: ann.timestamp,
  };
  if (endpoint) {
    res.endpoint = endpoint;
  }
  return res;
}

function normalizeTraceId(traceId) {
  if (traceId.length > 16) {
    return traceId.padStart(32, '0');
  }
  return traceId.padStart(16, '0');
}

// ported from zipkin2.v1.V1SpanConverter
function convertV1(span) {
  const res = {
    traceId: normalizeTraceId(span.traceId)
  };

  // take care not to create self-referencing spans even if the input data is incorrect
  const id = span.id.padStart(16, '0');
  if (span.parentId) {
    const parentId = span.parentId.padStart(16, '0');
    if (parentId !== id) {
      res.parentId = parentId;
    }
  }

  res.id = id;
  res.name = span.name || ''; // undefined is not allowed in v1
  if (span.debug) {
    res.debug = true;
  }

  // Don't report timestamp and duration on shared spans (should be server, but not necessarily)
  if (!span.shared) {
    if (span.timestamp) res.timestamp = span.timestamp;
    if (span.duration) res.duration = span.duration;
  }

  let startTs = span.timestamp || 0;
  let endTs = startTs && span.duration ? startTs + span.duration : 0;
  let msTs = 0;
  let wsTs = 0;
  let wrTs = 0;
  let mrTs = 0;

  let begin;
  let end;

  let kind = span.kind;

  // scan annotations in case there are better timestamps, or inferred kind
  (span.annotations || []).forEach((a) => {
    switch (a.value) {
      case 'cs':
        kind = 'CLIENT';
        if (a.timestamp < startTs) startTs = a.timestamp;
        break;
      case 'sr':
        kind = 'SERVER';
        if (a.timestamp < startTs) startTs = a.timestamp;
        break;
      case 'ss':
        kind = 'SERVER';
        if (a.timestamp > endTs) endTs = a.timestamp;
        break;
      case 'cr':
        kind = 'CLIENT';
        if (a.timestamp > endTs) endTs = a.timestamp;
        break;
      case 'ms':
        kind = 'PRODUCER';
        msTs = a.timestamp;
        break;
      case 'mr':
        kind = 'CONSUMER';
        mrTs = a.timestamp;
        break;
      case 'ws':
        wsTs = a.timestamp;
        break;
      case 'wr':
        wrTs = a.timestamp;
        break;
      default:
    }
  });

  let addr = 'sa'; // default which will be unset later if needed

  switch (kind) {
    case 'CLIENT':
      addr = 'sa';
      begin = 'cs';
      end = 'cr';
      break;
    case 'SERVER':
      addr = 'ca';
      begin = 'sr';
      end = 'ss';
      break;
    case 'PRODUCER':
      addr = 'ma';
      begin = 'ms';
      end = 'ws';
      if (startTs === 0 || (msTs !== 0 && msTs < startTs)) {
        startTs = msTs;
      }
      if (endTs === 0 || (wsTs !== 0 && wsTs > endTs)) {
        endTs = wsTs;
      }
      break;
    case 'CONSUMER':
      addr = 'ma';
      if (startTs === 0 || (wrTs !== 0 && wrTs < startTs)) {
        startTs = wrTs;
      }
      if (endTs === 0 || (mrTs !== 0 && mrTs > endTs)) {
        endTs = mrTs;
      }
      if (endTs !== 0 || wrTs !== 0) {
        begin = 'wr';
        end = 'mr';
      } else {
        begin = 'mr';
      }
      break;
    default:
  }

  // If we didn't find a span kind, directly or indirectly, unset the addr
  if (!span.remoteEndpoint) addr = undefined;

  const beginAnnotation = startTs && begin;
  const endAnnotation = endTs && end;
  const ep = toV1Endpoint(span.localEndpoint);

  res.annotations = []; // prefer empty to undefined for arrays

  let annotationCount = (span.annotations || []).length;
  if (beginAnnotation) {
    annotationCount++;
    res.annotations.push(toV1Annotation({
      value: begin,
      timestamp: startTs
    }, ep));
  }

  (span.annotations || []).forEach((a) => {
    if (beginAnnotation && a.value === begin) return;
    if (endAnnotation && a.value === end) return;
    res.annotations.push(toV1Annotation(a, ep));
  });

  if (endAnnotation) {
    annotationCount++;
    res.annotations.push(toV1Annotation({
      value: end,
      timestamp: endTs
    }, ep));
  }

  res.binaryAnnotations = []; // prefer empty to undefined for arrays
  const keys = Object.keys(span.tags || {});
  if (keys.length > 0) {
    res.binaryAnnotations = keys.map(key => ({
      key,
      value: span.tags[key],
      endpoint: ep
    }));
  }

  const writeLocalComponent = annotationCount === 0 && ep && keys.length === 0;
  const hasRemoteEndpoint = addr && span.remoteEndpoint;

  // write an empty 'lc' annotation to avoid missing the localEndpoint in an in-process span
  if (writeLocalComponent) {
    res.binaryAnnotations.push({key: 'lc', value: '', endpoint: ep});
  }
  if (hasRemoteEndpoint) {
    const address = {
      key: addr,
      value: true,
      endpoint: toV1Endpoint(span.remoteEndpoint)
    };
    res.binaryAnnotations.push(address);
  }

  return res;
}

// This guards to ensure we don't add duplicate annotations on merge
function maybePushAnnotation(annotations, a) {
  if (annotations.findIndex(b => a.value === b.value) === -1) {
    annotations.push(a);
  }
}

// This guards to ensure we don't add duplicate binary annotations on merge
function maybePushBinaryAnnotation(binaryAnnotations, a) {
  if (binaryAnnotations.findIndex(b => a.key === b.key) === -1) {
    binaryAnnotations.push(a);
  }
}

// This cleans potential dirty v1 inputs, like normalizing IDs etc.
function clean(span) {
  const res = {
    traceId: normalizeTraceId(span.traceId)
  };

  // take care not to create self-referencing spans even if the input data is incorrect
  const id = span.id.padStart(16, '0');
  if (span.parentId) {
    const parentId = span.parentId.padStart(16, '0');
    if (parentId !== id) {
      res.parentId = parentId;
    }
  }

  res.id = id;
  res.name = span.name || '';
  if (span.timestamp) res.timestamp = span.timestamp;
  if (span.duration) res.duration = span.duration;
  res.annotations = span.annotations || [];
  res.annotations.sort((a, b) => a.timestamp - b.timestamp);
  res.binaryAnnotations = span.binaryAnnotations || [];
  if (span.debug) {
    res.debug = true;
  }
  return res;
}

function merge(left, right) {
  // normalize ID lengths in case dirty input is received
  //  (this won't be the case from the normal zipkin server, as it normalizes IDs)
  const res = {
    traceId: normalizeTraceId(right.traceId.length > 16 ? right.traceId : left.traceId)
  };

  // take care not to create self-referencing spans even if the input data is incorrect
  const id = left.id.padStart(16, '0');
  if (left.parentId) {
    const leftParent = left.parentId.padStart(16, '0');
    if (leftParent !== id) {
      res.parentId = leftParent;
    }
  }

  if (right.parentId && !res.parentId) {
    const rightParent = right.parentId.padStart(16, '0');
    if (rightParent !== id) {
      res.parentId = rightParent;
    }
  }

  res.id = id;
  res.name = left.name || '';

  // When we move to span model 2, remove this code in favor of using Span.kind == CLIENT
  let leftClientSpan;
  let rightClientSpan;
  let rightServerSpan;

  res.annotations = [];

  (left.annotations || []).forEach((a) => {
    if (a.value === 'cs') leftClientSpan = true;
    maybePushAnnotation(res.annotations, a);
  });

  (right.annotations || []).forEach((a) => {
    if (a.value === 'cs') rightClientSpan = true;
    if (a.value === 'sr') rightServerSpan = true;
    maybePushAnnotation(res.annotations, a);
  });

  res.annotations.sort((a, b) => a.timestamp - b.timestamp);

  res.binaryAnnotations = [];

  (left.binaryAnnotations || []).forEach((b) => {
    maybePushBinaryAnnotation(res.binaryAnnotations, b);
  });

  (right.binaryAnnotations || []).forEach((b) => {
    maybePushBinaryAnnotation(res.binaryAnnotations, b);
  });

  if (right.name && right.name !== '' && right.name !== 'unknown') {
    if (res.name === '' || res.name === 'unknown') {
      res.name = right.name;
    } else if (leftClientSpan && rightServerSpan) {
      res.name = right.name; // prefer the server's span name
    }
  }

  // Single timestamp makes duration easy: just choose max
  if (!left.timestamp || !right.timestamp || left.timestamp === right.timestamp) {
    res.timestamp = left.timestamp || right.timestamp;
    if (!left.duration) {
      res.duration = right.duration;
    } else if (right.duration) {
      res.duration = Math.max(left.duration, right.duration);
    } else {
      res.duration = left.duration;
    }
  } else {
    // We have 2 different timestamps. If we have client data in either one of them, use right,
    // else set timestamp and duration to null
    if (rightClientSpan) {
      res.timestamp = right.timestamp;
      res.duration = right.duration;
    } else if (leftClientSpan) {
      res.timestamp = left.timestamp;
      res.duration = left.duration;
    }
  }

  if (right.debug) {
    res.debug = true;
  }
  return res;
}

/*
 * Instrumentation should set {@link Span#timestamp} when recording a span so that guess-work
 * isn't needed. Since a lot of instrumentation don't, we have to make some guesses.
 *
 * * If there is a 'cs', use that
 * * Fall back to 'sr'
 * * Otherwise, return undefined
 */
// originally zipkin.internal.ApplyTimestampAndDuration.guessTimestamp
function guessTimestamp(span) {
  if (span.timestamp || !span.annotations || span.annotations.length === 0) {
    return span.timestamp;
  }
  let rootServerRecv;
  for (let i = 0; i < span.annotations.length; i++) {
    const a = span.annotations[i];
    if (a.value === 'cs') {
      return a.timestamp;
    } else if (a.value === 'sr') {
      rootServerRecv = a.timestamp;
    }
  }
  return rootServerRecv;
}

/*
 * For RPC two-way spans, the duration between 'cs' and 'cr' is authoritative. RPC one-way spans
 * lack a response, so the duration is between 'cs' and 'sr'. We special-case this to avoid
 * setting incorrect duration when there's skew between the client and the server.
 */
// originally zipkin.internal.ApplyTimestampAndDuration.apply
function applyTimestampAndDuration(span) {
  // Don't overwrite authoritatively set timestamp and duration!
  if ((span.timestamp && span.duration) || !span.annotations) {
    return span;
  }

  // We cannot backfill duration on a span with less than two annotations. However, we can
  // backfill timestamp.
  const annotationLength = span.annotations.length;
  if (annotationLength < 2) {
    if (span.timestamp) return span;
    const guess = guessTimestamp(span);
    if (!guess) return span;
    span.timestamp = guess; // eslint-disable-line no-param-reassign
    return span;
  }

  // Prefer RPC one-way (cs -> sr) vs arbitrary annotations.
  let first = span.annotations[0].timestamp;
  let last = span.annotations[annotationLength - 1].timestamp;
  span.annotations.forEach((a) => {
    if (a.value === 'cs') {
      first = a.timestamp;
    } else if (a.value === 'cr') {
      last = a.timestamp;
    }
  });

  if (!span.timestamp) {
    span.timestamp = first; // eslint-disable-line no-param-reassign
  }
  if (!span.duration && last !== first) {
    span.duration = last - first; // eslint-disable-line no-param-reassign
  }
  return span;
}

// compares potentially undefined input
function compare(a, b) {
  if (!a && !b) return 0;
  if (!a) return -1;
  if (!b) return 1;
  return (a > b) - (a < b);
}

/*
 * v1 spans can be sent in multiple parts. Also client and server spans can share the same ID. This
 * merges both scenarios.
 */
// originally zipkin.internal.MergeById.apply
function mergeById(spans) {
  const result = [];

  if (!spans || spans.length === 0) return result;

  const spanIdToSpans = {};
  spans.forEach((s) => {
    const id = s.id.padStart(16, '0');
    spanIdToSpans[id] = spanIdToSpans[id] || [];
    spanIdToSpans[id].push(s);
  });

  Object.keys(spanIdToSpans).forEach(id => {
    const spansToMerge = spanIdToSpans[id];
    let left = clean(spansToMerge[0]);
    for (let i = 1; i < spansToMerge.length; i++) {
      left = merge(left, spansToMerge[i]);
    }

    // attempt to get a timestamp so that sorting will be helpful
    result.push(applyTimestampAndDuration(left));
  });

  // sort by timestamp, then name, root first in case of skew
  // TODO: this should be a topological sort
  return result.sort((a, b) => {
    if (!a.parentId) { // a is root
      return -1;
    } else if (!b.parentId) { // b is root
      return 1;
    }
    // Either a and b are root or neither are. In any case sort by timestamp, then name
    return compare(a.timestamp, b.timestamp) || compare(a.name, b.name);
  });
}

module.exports.SPAN_V1 = {
  convert(v2Span) {
    return convertV1(v2Span);
  },
  merge(v1Left, v1Right) {
    return merge(v1Left, v1Right);
  },
  applyTimestampAndDuration(v1Span) {
    return applyTimestampAndDuration(v1Span);
  },
  mergeById(v1Spans) {
    return mergeById(v1Spans);
  }
};
