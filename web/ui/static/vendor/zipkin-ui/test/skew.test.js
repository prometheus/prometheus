const {
  Node,
  TreeBuilder,
  ipsMatch,
  getClockSkew,
  isSingleHostSpan,
  correctForClockSkew
} = require('../js/skew');
const should = require('chai').should();

// originally zipkin2.internal.NodeTest.java
describe('Node', () => {
  it('should construct without a value', () => {
    const value = {traceId: '1', id: '1'};
    const node = new Node(value);

    expect(node.value).to.equal(value);
  });

  it('should construct without a value', () => {
    const node = new Node();

    should.equal(node.value, undefined);
  });

  it('should not allow setting an undefined value', () => {
    const node = new Node();

    expect(() => node.setValue()).to.throw('newValue was undefined');
  });

  it('should not allow creating a cycle', () => {
    const fake = new Node();

    expect(() => fake.addChild(fake)).to.throw('circular dependency on Node()');

    const node = new Node({traceId: '1', id: '1'});

    expect(() => node.addChild(node))
      .to.throw('circular dependency on Node({"traceId":"1","id":"1"})');
  });

  /*
   * The following tree should traverse in alphabetical order
   *
   *          a
   *        / | \
   *       b  c  d
   *      /|\     \
   *     e f g     h
   */
  it('should traverse breadth first', () => {
    const a = new Node('a');
    const b = new Node('b');
    const c = new Node('c');
    const d = new Node('d');
    // root(a) has children b, c, d
    a.addChild(b);
    a.addChild(c);
    a.addChild(d);
    const e = new Node('e');
    const f = new Node('f');
    const g = new Node('g');
    // child(b) has children e, f, g
    b.addChild(e);
    b.addChild(f);
    b.addChild(g);
    const h = new Node('h');
    // f has no children
    // child(g) has child h
    g.addChild(h);

    expect(a.traverse()).to.deep.equal([
      'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'
    ]);
  });
});

// originally zipkin2.internal.NodeTest.java
describe('TreeBuilder', () => {
  // Makes sure that the trace tree is constructed based on parent-child, not by parameter order.
  it('should construct a trace tree', () => {
    const trace = [
      {traceId: 'a', id: 'a'},
      {traceId: 'a', parentId: 'a', id: 'b'},
      {traceId: 'a', parentId: 'b', id: 'c'},
    ];

    const treeBuilder = new TreeBuilder({traceId: 'a'});

    // TRACE is sorted with root span first, lets reverse them to make
    // sure the trace is stitched together by id.
    trace.slice(0).reverse().forEach((span) => treeBuilder.addNode(span.parentId, span.id, span));

    const root = treeBuilder.build();
    expect(root.value).to.equal(trace[0]);
    expect(root.children.map(n => n.value)).to.deep.equal([trace[1]]);

    const child = root.children[0];
    expect(child.children.map(n => n.value)).to.deep.equal([trace[2]]);
  });

  // input should be merged, but this ensures we are fine anyway
  it('should dedupe while constructing a trace tree', () => {
    const trace = [
      {traceId: 'a', id: 'a'},
      {traceId: 'a', id: 'a'},
      {traceId: 'a', id: 'a'}
    ];

    const treeBuilder = new TreeBuilder({traceId: 'a'});
    trace.forEach((span) => treeBuilder.addNode(span.parentId, span.id, span));
    const root = treeBuilder.build();

    expect(root.value).to.equal(trace[0]);
    expect(root.children.length).to.equal(0);
  });

  it('should allocate spans missing parents to root', () => {
    const trace = [
      {traceId: 'a', id: 'b'},
      {traceId: 'a', parentId: 'b', id: 'c'},
      {traceId: 'a', parentId: 'b', id: 'd'},
      {traceId: 'a', id: 'e'},
      {traceId: 'a', id: 'f'}
    ];

    const treeBuilder = new TreeBuilder({traceId: 'a'});
    trace.forEach((span) => treeBuilder.addNode(span.parentId, span.id, span));
    const root = treeBuilder.build();

    expect(root.traverse()).to.deep.equal(trace);
    expect(root.children.map(n => n.value)).to.deep.equal(trace.slice(1));
  });

  // spans are often reported depth-first, so it is possible to not have a root yet
  it('should construct a trace missing a root span', () => {
    const trace = [
      {traceId: 'a', parentId: 'a', id: 'b'},
      {traceId: 'a', parentId: 'a', id: 'c'},
      {traceId: 'a', parentId: 'a', id: 'd'}
    ];

    const treeBuilder = new TreeBuilder({traceId: 'a'});
    trace.forEach((span) => treeBuilder.addNode(span.parentId, span.id, span));
    const root = treeBuilder.build();

    should.equal(root.value, undefined);
    expect(root.traverse()).to.deep.equal(trace);
  });

  // input should be well formed, but this ensures we are fine anyway
  it('should skip on cycle', () => {
    const treeBuilder = new TreeBuilder({traceId: 'a'});
    expect(treeBuilder.addNode('b', 'b', {traceId: 'a', parentId: 'b', id: 'b'}))
      .to.equal(false);
  });
});

const networkLatency = 10;

function createRootSpan(endpoint, begin, duration) {
  return {
    traceId: '1',
    id: '1',
    timestamp: begin,
    annotations: [
      {timestamp: begin, value: 'sr', endpoint},
      {timestamp: begin + duration, value: 'ss', endpoint}
    ]
  };
}

function childSpan(parent, to, begin, duration, skew) {
  const spanId = parent.id + 1;
  const from = parent.annotations[0].endpoint;
  return {
    traceId: parent.traceId,
    parentId: parent.id,
    id: spanId,
    timestamp: begin,
    annotations: [
      {timestamp: begin, value: 'cs', endpoint: from},
      {timestamp: begin + skew + networkLatency, value: 'sr', endpoint: to},
      {timestamp: begin + skew + duration - networkLatency, value: 'ss', endpoint: to},
      {timestamp: begin + duration, value: 'cr', endpoint: from}
    ]
  };
}

function localSpan(parent, endpoint, begin, duration) {
  const spanId = parent.id + 1;
  return {
    traceId: parent.traceId,
    parentId: parent.id,
    id: spanId,
    timestamp: begin,
    duration,
    binaryAnnotations: [{key: 'lc', value: '', endpoint}]
  };
}

function annotationTimestamp(span, annotation) {
  return span.annotations.find(a => a.value === annotation).timestamp;
}

// originally zipkin2.internal.CorrectForClockSkewTest.java
describe('correctForClockSkew', () => {
  // endpoints from zipkin2.TestObjects
  const frontend = {
    serviceName: 'frontend',
    ipv4: '127.0.0.1',
    port: 8080
  };

  const backend = {
    serviceName: 'backend',
    ipv4: '192.168.99.101',
    port: 9000
  };

  const db = {
    serviceName: 'db',
    ipv4: '2001:db8::c001',
    port: 3306
  };

  const ipv6 = {ipv6: '2001:db8::c001'};
  const ipv4 = {ipv4: '192.168.99.101'};
  const both = {
    ipv4: '192.168.99.101',
    ipv6: '2001:db8::c001'
  };

  it('IPs should not match unless both sides have an IP', () => {
    const noIp = {serviceName: 'foo'};
    expect(ipsMatch(noIp, ipv4)).to.equal(false);
    expect(ipsMatch(noIp, ipv6)).to.equal(false);
    expect(ipsMatch(ipv4, noIp)).to.equal(false);
    expect(ipsMatch(ipv6, noIp)).to.equal(false);
  });

  it('IPs should not match when IPs are different', () => {
    const differentIpv6 = {ipv6: '2001:db8::c002'};
    const differentIpv4 = {ipv4: '192.168.99.102'};

    expect(ipsMatch(differentIpv4, ipv4)).to.equal(false);
    expect(ipsMatch(differentIpv6, ipv6)).to.equal(false);
    expect(ipsMatch(ipv4, differentIpv4)).to.equal(false);
    expect(ipsMatch(ipv6, differentIpv6)).to.equal(false);
  });

  it('IPs should match when ipv4 or ipv6 match', () => {
    expect(ipsMatch(ipv4, ipv4)).to.equal(true);
    expect(ipsMatch(both, ipv4)).to.equal(true);
    expect(ipsMatch(both, ipv6)).to.equal(true);
    expect(ipsMatch(ipv6, ipv6)).to.equal(true);
    expect(ipsMatch(ipv4, both)).to.equal(true);
    expect(ipsMatch(ipv6, both)).to.equal(true);
  });

  /*
   * Instrumentation bugs might result in spans that look like clock skew is at play. When skew
   * appears on the same host, we assume it is an instrumentation bug (rather than make it worse by
   * adjusting it!)
   */
  it('clock skew should only correct across different hosts', () => {
    const skewedSameHost = {
      traceId: '1',
      id: '1',
      annotations: [
        {timestamp: 20, value: 'cs', endpoint: frontend},
        {timestamp: 10 /* skew */, value: 'sr', endpoint: frontend},
        {timestamp: 20, value: 'ss', endpoint: frontend},
        {timestamp: 40, value: 'cr', endpoint: frontend}
      ]
    };
    should.equal(getClockSkew(skewedSameHost), undefined);
  });

  /*
   * Skew is relative to the server receive and centered by the difference between the server
   * duration and the client duration.
   */
  it('skew includes half the difference of client and server duration', () => {
    const cs = {timestamp: 20, value: 'cs', endpoint: frontend};
    const sr = {timestamp: 10 /* skew */, value: 'sr', endpoint: backend};
    const ss = {timestamp: 20, value: 'ss', endpoint: backend};
    const cr = {timestamp: 40, value: 'cr', endpoint: frontend};
    const skewedRpc = {traceId: '1', id: '1', annotations: [cs, sr, ss, cr]};

    const skew = getClockSkew(skewedRpc);

    // Skew correction pushes the server side forward, so the skew endpoint is the server
    expect(skew.endpoint).to.equal(sr.endpoint);

    // client duration = 20, server duration = 10: center the server by splitting what's left
    const clientDuration = cr.timestamp - cs.timestamp;
    const serverDuration = ss.timestamp - sr.timestamp;
    expect(skew.skew).to.equal(
      sr.timestamp - cs.timestamp // how much sr is behind
      - (clientDuration - serverDuration) / 2 // center the server by splitting what's left
    );
  });

  // Sets the server to 1us past the client
  it('skew on one-way spans assumes latency is at least 1us', () => {
    const cs = {timestamp: 20, value: 'cs', endpoint: frontend};
    const sr = {timestamp: 10 /* skew */, value: 'sr', endpoint: backend};
    const skewedOneWay = {traceId: '1', id: '1', annotations: [cs, sr]};

    const skew = getClockSkew(skewedOneWay);

    expect(skew.skew).to.equal(
      sr.timestamp - cs.timestamp // how much sr is behind
      - 1 // assume it takes at least 1us to get to the server
    );
  });

  // It is still impossible to reach the server before the client sends a request.
  it('skew on async server spans assumes latency is at least 1us', () => {
    const cs = {timestamp: 20, value: 'cs', endpoint: frontend};
    const sr = {timestamp: 10 /* skew */, value: 'sr', endpoint: backend};
    const ss = {timestamp: 20, value: 'ss', endpoint: backend}; // server latency is double client
    const cr = {timestamp: 25, value: 'cr', endpoint: frontend};
    const skewedAsyncRpc = {traceId: '1', id: '1', annotations: [cs, sr, ss, cr]};

    const skew = getClockSkew(skewedAsyncRpc);

    expect(skew.skew).to.equal(
      sr.timestamp - cs.timestamp // how much sr is behind
      - 1 // assume it takes at least 1us to get to the server
    );
  });

  it('clock skew requires endpoints', () => {
    const skewedButNoendpoints = {
      traceId: '1',
      id: '1',
      annotations: [
        {timestamp: 20, value: 'cs'},
        {timestamp: 10 /* skew */, value: 'sr'},
        {timestamp: 20, value: 'ss'},
        {timestamp: 40, value: 'cr'}
      ]
    };
    should.equal(getClockSkew(skewedButNoendpoints), undefined);
  });

  it('span with the mixed endpoints is not single-host', () => {
    const span = {
      traceId: '1',
      id: '1',
      annotations: [
        {timestamp: 20, value: 'cs', endpoint: frontend},
        {timestamp: 40, value: 'sr', endpoint: backend}
      ]
    };

    expect(isSingleHostSpan(span)).to.equal(false);
  });

  it('span with the same endpoint is single-host', () => {
    const span = {
      traceId: '1',
      id: '1',
      annotations: [
        {timestamp: 20, value: 'cs', endpoint: frontend},
        {timestamp: 40, value: 'cr', endpoint: frontend}
      ]
    };

    expect(isSingleHostSpan(span)).to.equal(true);
  });

  it('span with lc binary annotation is single-host', () => {
    const span = {
      traceId: '1',
      id: '1',
      binaryAnnotations: [{key: 'lc', value: '', endpoint: frontend}]
    };

    expect(isSingleHostSpan(span)).to.equal(true);
  });

  // Sets the server to 1us past the client
  it('should correct a one-way RPC span', () => {
    const trace = [
      {
        traceId: '1',
        id: '1',
        annotations: [
          {timestamp: 20, value: 'cs', endpoint: frontend},
          {timestamp: 10 /* skew */, value: 'sr', endpoint: backend}
        ]
      }
    ];

    const adjusted = correctForClockSkew(trace);

    expect(adjusted.length).to.equal(1);
    expect(adjusted[0].annotations).to.deep.equal([
      {timestamp: 20, value: 'cs', endpoint: frontend},
      {timestamp: 21 /* pushed 1us later */, value: 'sr', endpoint: backend}
    ]);
  });

  function assertClockSkewIsCorrectlyApplied(skew) {
    const rpcClientSendTs = 50;
    const dbClientSendTimestamp = 60 + skew;

    const rootDuration = 350;
    const rpcDuration = 250;
    const dbDuration = 40;

    const rootSpan = createRootSpan(frontend, 0, rootDuration);
    const rpcSpan = childSpan(rootSpan, backend, rpcClientSendTs, rpcDuration, skew);
    const tierSpan = childSpan(rpcSpan, db, dbClientSendTimestamp, dbDuration, skew);

    const adjustedSpans = correctForClockSkew([rootSpan, rpcSpan, tierSpan]);

    const adjustedRpcSpan = adjustedSpans.find((s) => s.id === rpcSpan.id);
    expect(annotationTimestamp(adjustedRpcSpan, 'sr')).to.equal(rpcClientSendTs + networkLatency);
    expect(annotationTimestamp(adjustedRpcSpan, 'cs')).to.equal(adjustedRpcSpan.timestamp);

    const adjustedTierSpan = adjustedSpans.find((s) => s.id === tierSpan.id);
    expect(annotationTimestamp(adjustedTierSpan, 'cs')).to.equal(adjustedTierSpan.timestamp);
  }

  it('should correct an rpc when server sends after client receive', () => {
    assertClockSkewIsCorrectlyApplied(50000);
  });

  it('should correct an rpc when server sends after client send', () => {
    assertClockSkewIsCorrectlyApplied(-50000);
  });

  it('should propagate skew to local spans', () => {
    const rootSpan = createRootSpan(frontend, 0, 2000);
    const skew = -50000;
    const rpcSpan = childSpan(rootSpan, backend, networkLatency, 1000, skew);
    const local = localSpan(rpcSpan, backend, rpcSpan.timestamp + 5, 200);
    const local2 = localSpan(local, backend, local.timestamp + 10, 100);
    const local3 = { // missing timestamp duration and even missing endpoint
      traceId: local2.traceId,
      parentId: local2.id,
      id: local2.id + 1,
      binaryAnnotations: [{key: 'lc', value: ''}]
    };

    const adjustedSpans = correctForClockSkew([rootSpan, rpcSpan, local, local2, local3]);

    const adjustedLocal = adjustedSpans.find((s) => s.id === local.id);
    expect(adjustedLocal.timestamp).to.equal(local.timestamp - skew);

    const adjustedLocal2 = adjustedSpans.find((s) => s.id === local2.id);
    expect(adjustedLocal2.timestamp).to.equal(local2.timestamp - skew);

    const adjustedLocal3 = adjustedSpans.find((s) => s.id === local3.id);
    expect(adjustedLocal3).to.equal(local3); // no change
  });

  // instrumentation errors can result in mixed spans
  it('should not propagate skew past mixed endpoint local spans', () => {
    const rootSpan = createRootSpan(frontend, 0, 2000);
    const skew = -50000;
    const rpcSpan = childSpan(rootSpan, backend, networkLatency, 1000, skew);
    const mixed = {
      traceId: rpcSpan.traceId,
      parentId: rpcSpan.id,
      id: rpcSpan.id + 1,
      timestamp: rpcSpan.timestamp + 5,
      duration: 200,
      binaryAnnotations: [
        {key: 'lc', value: '', endpoint: frontend},
        {key: 'lc', value: '', endpoint: backend} // invalid!
      ]
    };
    const childOfMixed = localSpan(mixed, backend, mixed.timestamp + 10, 100);

    const adjustedSpans = correctForClockSkew([rootSpan, rpcSpan, mixed, childOfMixed]);

    // mixed gets the skew correction
    const adjustedMixed = adjustedSpans.find((s) => s.id === mixed.id);
    expect(adjustedMixed.timestamp).to.equal(mixed.timestamp - skew);

    // its child does not
    const adjustedChildOfMixed = adjustedSpans.find((s) => s.id === childOfMixed.id);
    expect(adjustedChildOfMixed).to.equal(childOfMixed);
  });

  // a v1 span missing RPC annotations and also missing an "lc" tag shouldn't be affected
  it('should not propagate skew to unidentified spans', () => {
    const rootSpan = createRootSpan(frontend, 0, 2000);
    const skew = -50000;
    const rpcSpan = childSpan(rootSpan, backend, networkLatency, 1000, skew);
    const notRpcNotLc = {
      traceId: rpcSpan.traceId,
      parentId: rpcSpan.id,
      id: rpcSpan.id + 1,
      timestamp: rpcSpan.timestamp + 5,
      duration: 200,
      binaryAnnotations: [
        {key: 'foo', value: 'bar', endpoint: frontend} // not lc
      ]
    };

    const adjustedSpans = correctForClockSkew([rootSpan, rpcSpan, notRpcNotLc]);

    // ambiguous span is left alone
    const adjustedNotRpcNotLc = adjustedSpans.find((s) => s.id === notRpcNotLc.id);
    expect(adjustedNotRpcNotLc).to.equal(notRpcNotLc);
  });

  it('should skip adjustment on missing root span', () => {
    const trace = [
      {
        traceId: '1',
        id: '2',
        parentId: '1',
        annotations: [
          {timestamp: 20, value: 'cs', endpoint: frontend},
          {timestamp: 10 /* skew */, value: 'sr', endpoint: backend}
        ]
      }
    ];

    const adjusted = correctForClockSkew(trace);

    expect(adjusted).to.deep.equal(trace);
  });

  it('should skip adjustment on duplicate root span', () => {
    const trace = [
      {
        traceId: '1',
        id: '1',
        annotations: [
          {timestamp: 20, value: 'cs', endpoint: frontend},
          {timestamp: 10 /* skew */, value: 'sr', endpoint: backend}
        ]
      },
      {
        traceId: '1',
        id: '2', // missing parent ID
        annotations: [
          {timestamp: 20, value: 'cs', endpoint: frontend},
          {timestamp: 10 /* skew */, value: 'sr', endpoint: backend}
        ]
      }
    ];

    const adjusted = correctForClockSkew(trace);

    expect(adjusted).to.deep.equal(trace);
  });

  it('should skip adjustment on cycle', () => {
    const trace = [
      {
        traceId: '1',
        id: '1',
        parentId: '1',
        annotations: [
          {timestamp: 20, value: 'cs', endpoint: frontend},
          {timestamp: 10 /* skew */, value: 'sr', endpoint: backend}
        ]
      }
    ];

    const adjusted = correctForClockSkew(trace);
    expect(adjusted).to.deep.equal(trace);
  });
});
