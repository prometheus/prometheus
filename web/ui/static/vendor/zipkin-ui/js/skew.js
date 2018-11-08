/*
 * Convenience type representing a tree. This is here because multiple facets in zipkin require
 * traversing the trace tree. For example, looking at network boundaries to correct clock skew, or
 * counting requests imply visiting the tree.
 */
// originally zipkin2.internal.Node.java
class Node {
  constructor(value) {
    this._parent = undefined; // no default
    this._value = value; // undefined is possible when this is a synthetic root node
    this._children = [];
    this._missingRootDummyNode = false;
  }

  // Returns the parent, or undefined if root.
  get parent() {
    return this._parent;
  }

  // Returns the value, or undefined if a synthetic root node
  get value() {
    return this._value;
  }

  // Returns the children of this node
  get children() {
    return this._children;
  }

  // Mutable as some transformations, such as clock skew, adjust the current node in the tree.
  setValue(newValue) {
    if (!newValue) throw new Error('newValue was undefined');
    this._value = newValue;
  }

  _setParent(newParent) {
    this._parent = newParent;
  }

  addChild(child) {
    if (child === this) throw new Error(`circular dependency on ${this.toString()}`);
    child._setParent(this);
    this._children.push(child);
  }

  // Returns an array of values resulting from a breadth-first traversal at this node
  traverse() {
    const result = [];
    const queue = [this];

    while (queue.length > 0) {
      const current = queue.shift();

      // when there's a synthetic root span, the value could be undefined
      if (current.value) result.push(current.value);

      const children = current.children;
      for (let i = 0; i < children.length; i++) {
        queue.push(children[i]);
      }
    }
    return result;
  }

  toString() {
    if (this._value) return `Node(${JSON.stringify(this._value)})`;
    return 'Node()';
  }
}

/*
 * Some operations do not require the entire span object. This creates a tree given (parent id,
 * id) pairs.
 */
class TreeBuilder {
  constructor(params) {
    const {traceId, debug = false} = params;
    this._mergeFunction = (existing, update) => existing || update; // first non null
    if (!traceId) throw new Error('traceId was undefined');
    this._traceId = traceId;
    this._debug = debug;
    this._rootId = undefined;
    this._rootNode = undefined;
    this._entries = [];
      // Nodes representing the trace tree
    this._idToNode = {};
      // Collect the parent-child relationships between all spans.
    this._idToParent = {};
  }

  // Returns false after logging on debug if the value couldn't be added
  addNode(parentId, id, value) {
    if (parentId && parentId === id) {
      if (this._debug) {
        /* eslint-disable no-console */
        console.log(`skipping circular dependency: traceId=${this._traceId}, spanId=${id}`);
      }
      return false;
    }
    this._idToParent[id] = parentId;
    this._entries.push({parentId, id, value});
    return true;
  }

  _processNode(entry) {
    let parentId = entry.parentId ? entry.parentId : this._idToParent[entry.id];
    const id = entry.id;
    const value = entry.value;

    if (!parentId) {
      if (this._rootId) {
        if (this._debug) {
          const prefix = 'attributing span missing parent to root';
          /* eslint-disable no-console */
          console.log(
            `${prefix}: traceId=${this._traceId}, rootSpanId=${this._rootId}, spanId=${id}`
          );
        }
        parentId = this._rootId;
        this._idToParent[id] = parentId;
      } else {
        this._rootId = id;
      }
    }

    const node = new Node(value);
    // special-case root, and attribute missing parents to it. In
    // other words, assume that the first root is the "real" root.
    if (!parentId && !this._rootNode) {
      this._rootNode = node;
      this._rootId = id;
    } else if (!parentId && this._rootId === id) {
      this._rootNode.setValue(this._mergeFunction(this._rootNode.value, node.value));
    } else {
      const previous = this._idToNode[id];
      this._idToNode[id] = node;
      if (previous) node.setValue(this._mergeFunction(previous.value, node.value));
    }
  }

  // Builds a tree from calls to addNode, or returns an empty tree.
  build() {
    this._entries.forEach((n) => this._processNode(n));
    if (!this._rootNode) {
      if (this._debug) {
        /* eslint-disable no-console */
        console.log(`substituting dummy node for missing root span: traceId=${this._traceId}`);
      }
      this._rootNode = new Node();
    }

    // Materialize the tree using parent - child relationships
    Object.keys(this._idToParent).forEach(id => {
      if (id === this._rootId) return; // don't re-process root

      const node = this._idToNode[id];
      const parent = this._idToNode[this._idToParent[id]];
      if (!parent) { // handle headless
        this._rootNode.addChild(node);
      } else {
        parent.addChild(node);
      }
    });
    return this._rootNode;
  }
}

class ClockSkew {
  constructor(params) {
    const {endpoint, skew} = params;
    this._endpoint = endpoint;
    this._skew = skew;
  }

  get endpoint() {
    return this._endpoint;
  }

  get skew() {
    return this._skew;
  }
}

function ipsMatch(a, b) {
  if (!a || !b) return false;

  if (a.ipv6 && b.ipv6 && a.ipv6 === b.ipv6) {
    return true;
  }
  if (!a.ipv4 && !b.ipv4) return false;
  return a.ipv4 === b.ipv4;
}

// If any annotation has an IP with skew associated, adjust accordingly.
function adjustTimestamps(span, skew) {
  const spanTimestamp = span.timestamp;

  let annotations;
  let annotationTimestamp;
  const annotationLength = span.annotations ? span.annotations.length : 0;
  for (let i = 0; i < annotationLength; i++) {
    const a = span.annotations[i];
    if (!a.endpoint) continue;
    if (ipsMatch(skew.endpoint, a.endpoint)) {
      if (!annotations) annotations = span.annotations.slice(0);
      if (spanTimestamp && a.timestamp === spanTimestamp) {
        annotationTimestamp = a.timestamp;
      }
      annotations[i] = {timestamp: a.timestamp - skew.skew, value: a.value, endpoint: a.endpoint};
    }
  }
  if (annotations) {
    const result = Object.assign({}, span);
    if (annotationTimestamp) {
      result.timestamp = annotationTimestamp - skew.skew;
    }
    result.annotations = annotations;
    return result;
  }
  // Search for a local span on the skewed endpoint
  if (!spanTimestamp) return span; // We can't adjust something lacking a timestamp
  const binaryAnnotations = span.binaryAnnotations || [];
  for (let i = 0; i < binaryAnnotations.length; i++) {
    const b = binaryAnnotations[i];
    if (!b.endpoint) continue;
    if (b.key === 'lc' && ipsMatch(skew.endpoint, b.endpoint)) {
      const result = Object.assign({}, span);
      result.timestamp = spanTimestamp - skew.skew;
      return result;
    }
  }
  return span;
}

function oneWaySkew(serverRecv, clientSend) {
  const latency = serverRecv.timestamp - clientSend.timestamp;
  // the only way there is skew is when the client appears to be after the server
  if (latency > 0) return undefined;
  // We can't currently do better than push the client and server apart by minimum duration (1)
  return new ClockSkew({endpoint: serverRecv.endpoint, skew: latency - 1});
}

// Uses client/server annotations to determine if there's clock skew.
function getClockSkew(span) {
  let clientSend;
  let serverRecv;
  let serverSend;
  let clientRecv;

  (span.annotations || []).forEach((a) => {
    switch (a.value) {
      case 'cs':
        clientSend = a;
        break;
      case 'sr':
        serverRecv = a;
        break;
      case 'ss':
        serverSend = a;
        break;
      case 'cr':
        clientRecv = a;
        break;
      default:
    }
  });

  let oneWay = false;
  if (!clientSend || !serverRecv) {
    return undefined;
  } else if (!serverSend || !clientRecv) {
    oneWay = true;
  }

  let server = serverRecv.endpoint;
  if (!server && oneWay) server = serverSend.endpoint;
  if (!server) return undefined;

  let client = clientSend.endpoint;
  if (!client && oneWay) client = clientRecv.endpoint;
  if (!client) return undefined;

  // There's no skew if the RPC is going to itself
  if (ipsMatch(server, client)) return undefined;

  let latency;
  if (oneWay) {
    return oneWaySkew(serverRecv, clientSend);
  } else {
    const clientDuration = clientRecv.timestamp - clientSend.timestamp;
    const serverDuration = serverSend.timestamp - serverRecv.timestamp;
    // In best case, we assume latency is half the difference between the client and server
    // duration.
    //
    // If the server duration is longer than client, we can't do that: so we use the same approach
    // as one-way.
    if (clientDuration < serverDuration) {
      return oneWaySkew(serverRecv, clientSend);
    }

    latency = (clientDuration - serverDuration) / 2;
    // We can't see skew when send happens before receive
    if (latency < 0) return undefined;

    const skew = serverRecv.timestamp - latency - clientSend.timestamp;
    if (skew !== 0) {
      return new ClockSkew({endpoint: server, skew});
    }
  }

  return undefined;
}

function isSingleHostSpan(span) {
  const annotations = span.annotations || [];
  const binaryAnnotations = span.binaryAnnotations || [];

  // using normal for loop as it allows us to return out of the function
  let endpoint;
  for (let i = 0; i < annotations.length; i++) {
    const annotation = annotations[i];
    if (!endpoint) {
      endpoint = annotation.endpoint;
      continue;
    }
    if (!ipsMatch(endpoint, annotation.endpoint)) {
      return false; // there's a mix of endpoints in this span
    }
  }
  for (let i = 0; i < binaryAnnotations.length; i++) {
    const binaryAnnotation = binaryAnnotations[i];
    if (binaryAnnotation.type || binaryAnnotation.value === true) continue;
    if (!endpoint) {
      endpoint = binaryAnnotation.endpoint;
      continue;
    }
    if (!ipsMatch(endpoint, binaryAnnotation.endpoint)) {
      return false; // there's a mix of endpoints in this span
    }
  }
  return true;
}

/*
 * Recursively adjust the timestamps on the span tree. Root span is the reference point, all
 * children's timestamps gets adjusted based on that span's timestamps.
 */
function adjust(node, skewFromParent) {
  // adjust skew for the endpoint brought over from the parent span
  if (skewFromParent) {
    node.setValue(adjustTimestamps(node.value, skewFromParent));
  }

  // Is there any skew in the current span?
  let skew = getClockSkew(node.value);
  if (skew) {
    // the current span's skew may be a different endpoint than its parent, so adjust again.
    node.setValue(adjustTimestamps(node.value, skew));
  } else if (skewFromParent && isSingleHostSpan(node.value)) {
    // Assumes we are on the same host: propagate skew from our parent
    skew = skewFromParent;
  }
  // propagate skew to any children
  node.children.forEach(child => adjust(child, skew));
}

function correctForClockSkew(spans, debug = false) {
  if (spans.length === 0) return spans;

  const traceId = spans[0].traceId;
  let rootSpanId;
  const treeBuilder = new TreeBuilder({traceId, debug});

  let dataError = false;
  spans.forEach(next => {
    if (!next.parentId) {
      if (rootSpanId) {
        if (debug) {
          const prefix = 'skipping redundant root span';
          /* eslint-disable no-console */
          console.log(
            `${prefix}: traceId=${traceId}, rootSpanId=${rootSpanId}, spanId=${next.id}`
          );
        }
        dataError = true;
        return;
      }
      rootSpanId = next.id;
    }
    if (!treeBuilder.addNode(next.parentId, next.id, next)) {
      dataError = true;
    }
  });

  if (!rootSpanId) {
    if (debug) {
      console.log(`skipping clock skew adjustment due to missing root span: traceId=${traceId}`);
    }
    return spans;
  } else if (dataError) {
    if (debug) {
      console.log(`skipping clock skew adjustment due to data errors: traceId=${traceId}`);
    }
    return spans;
  }

  const tree = treeBuilder.build();
  adjust(tree);
  return tree.traverse();
}


module.exports = {
  Node,
  TreeBuilder,
  ipsMatch, // for testing
  isSingleHostSpan, // for testing
  getClockSkew, // for testing
  correctForClockSkew
};
