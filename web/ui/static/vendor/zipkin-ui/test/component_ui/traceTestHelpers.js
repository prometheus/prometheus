export function endpoint(ipv4, port, serviceName) {
  return {ipv4, port, serviceName};
}

export function annotation(timestamp, value, ep) {
  return {timestamp, value, endpoint: ep};
}

export function binaryAnnotation(key, value, ep) {
  return {key, value, endpoint: ep};
}

export function span(traceId,
                     name,
                     id,
                     parentId = null,
                     timestamp = null,
                     duration = null,
                     annotations = [],
                     binaryAnnotations = [],
                     debug = false) {
  return {
    traceId,
    name,
    id,
    parentId,
    timestamp,
    duration,
    annotations,
    binaryAnnotations,
    debug
  };
}

export function traceDetailSpan(id) {
  const expanderText = [];
  const classes = new Set();
  return {
    id,
    inFilters: 0,
    openParents: 0,
    openChildren: 0,
    shown: false,
    show() {this.shown = true;},
    hidden: false,
    hide() {this.hidden = true;},
    expanderText,
    expanded: false,
    $expander: {text: t => expanderText.push(t)},
    classes,
    addClass: c => classes.add(c),
    removeClass: c => classes.delete(c)
  };
}
