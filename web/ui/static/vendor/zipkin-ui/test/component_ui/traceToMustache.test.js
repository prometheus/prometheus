import {Constants} from '../../js/component_ui/traceConstants';
import traceToMustache,
  {
    getRootSpans,
    formatEndpoint
  } from '../../js/component_ui/traceToMustache';
import {endpoint, annotation, span} from './traceTestHelpers';

const ep1 = endpoint(123, 123, 'service1');
const ep2 = endpoint(456, 456, 'service2');
const ep3 = endpoint(666, 666, 'service2');
const ep4 = endpoint(777, 777, 'service3');
const ep5 = endpoint(888, 888, 'service3');

const annotations1 = [
  annotation(100, Constants.CLIENT_SEND, ep1),
  annotation(150, Constants.CLIENT_RECEIVE, ep1)
];
const annotations2 = [
  annotation(200, Constants.CLIENT_SEND, ep2),
  annotation(250, Constants.CLIENT_RECEIVE, ep2)
];
const annotations3 = [
  annotation(300, Constants.CLIENT_SEND, ep2),
  annotation(350, Constants.CLIENT_RECEIVE, ep3)
];
const annotations4 = [
  annotation(400, Constants.CLIENT_SEND, ep4),
  annotation(500, Constants.CLIENT_RECEIVE, ep5)
];

const span1Id = '666';
const span2Id = '777';
const span3Id = '888';
const span4Id = '999';

const span1 = span(12345, 'methodcall1', span1Id, null, 100, 50, annotations1);
const span2 = span(12345, 'methodcall2', span2Id, span1Id, 200, 50, annotations2);
const span3 = span(12345, 'methodcall2', span3Id, span2Id, 300, 50, annotations3);
const span4 = span(12345, 'methodcall2', span4Id, span3Id, 400, 100, annotations4);

const trace = [span1, span2, span3, span4];

describe('traceToMustache', () => {
  it('should format duration', () => {
    const modelview = traceToMustache(trace);
    modelview.duration.should.equal('400μ');
  });

  it('should show the number of services', () => {
    const modelview = traceToMustache(trace);
    modelview.services.should.equal(3);
  });

  it('should show logsUrl', () => {
    const logsUrl = 'http/url.com';
    const modelview = traceToMustache(trace, logsUrl);
    modelview.logsUrl.should.equal(logsUrl);
  });

  it('should show service counts', () => {
    const modelview = traceToMustache(trace);
    modelview.serviceCounts.should.eql([{
      name: 'service1',
      count: 1,
      max: 0
    }, {
      name: 'service2',
      count: 2,
      max: 0
    }, {
      name: 'service3',
      count: 1,
      max: 0
    }]);
  });

  it('should show human-readable annotation name', () => {
    const testTrace = [{
      traceId: '2480ccca8df0fca5',
      name: 'get',
      id: '2480ccca8df0fca5',
      timestamp: 1457186385375000,
      duration: 333000,
      annotations: [{
        timestamp: 1457186385375000,
        value: 'sr',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }, {
        timestamp: 1457186385708000,
        value: 'ss',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }],
      binaryAnnotations: [{
        key: 'sa',
        value: true,
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }, {
        key: 'literally-false',
        value: 'false',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }]
    }];
    const {spans: [testSpan]} = traceToMustache(testTrace);
    testSpan.annotations[0].value.should.equal('Server Receive');
    testSpan.annotations[1].value.should.equal('Server Send');
    testSpan.binaryAnnotations[0].key.should.equal('Server Address');
    testSpan.binaryAnnotations[1].value.should.equal('false');
  });

  it('should tolerate spans without annotations', () => {
    const testTrace = [{
      traceId: '2480ccca8df0fca5',
      name: 'get',
      id: '2480ccca8df0fca5',
      timestamp: 1457186385375000,
      duration: 333000,
      binaryAnnotations: [{
        key: 'lc',
        value: 'component',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }]
    }];
    const {spans: [testSpan]} = traceToMustache(testTrace);
    testSpan.binaryAnnotations[0].key.should.equal('Local Component');
  });

  it('should not include empty Local Component annotations', () => {
    const testTrace = [{
      traceId: '2480ccca8df0fca5',
      name: 'get',
      id: '2480ccca8df0fca5',
      timestamp: 1457186385375000,
      duration: 333000,
      binaryAnnotations: [{
        key: 'lc',
        value: '',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }]
    }];
    const {spans: [testSpan]} = traceToMustache(testTrace);
    // skips empty Local Component, but still shows it as an address
    testSpan.binaryAnnotations[0].key.should.equal('Local Address');
  });

  it('should tolerate spans without binary annotations', () => {
    const testTrace = [{
      traceId: '2480ccca8df0fca5',
      name: 'get',
      id: '2480ccca8df0fca5',
      timestamp: 1457186385375000,
      duration: 333000,
      annotations: [{
        timestamp: 1457186385375000,
        value: 'sr',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }, {
        timestamp: 1457186385708000,
        value: 'ss',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }]
    }];
    const {spans: [testSpan]} = traceToMustache(testTrace);
    testSpan.annotations[0].value.should.equal('Server Receive');
    testSpan.annotations[1].value.should.equal('Server Send');
  });
});

describe('get root spans', () => {
  it('should find root spans in a trace', () => {
    const testTrace = [{
      parentId: null, // root span (no parent)
      id: 1
    }, {
      parentId: 1,
      id: 2
    }, {
      parentId: 3, // root span (no parent with this id)
      id: 4
    }, {
      parentId: 4,
      id: 5
    }];

    const rootSpans = getRootSpans(testTrace);
    rootSpans.should.eql([{
      parentId: null,
      id: 1
    }, {
      parentId: 3,
      id: 4
    }]);
  });
});

describe('formatEndpoint', () => {
  it('should format ip and port', () => {
    formatEndpoint({ipv4: '150.151.152.153', port: 5000}).should.equal('150.151.152.153:5000');
  });

  it('should not use port when missing or zero', () => {
    formatEndpoint({ipv4: '150.151.152.153'}).should.equal('150.151.152.153');
    formatEndpoint({ipv4: '150.151.152.153', port: 0}).should.equal('150.151.152.153');
  });

  it('should put service name in parenthesis', () => {
    formatEndpoint({ipv4: '150.151.152.153', port: 9042, serviceName: 'cassandra'}).should.equal(
      '150.151.152.153:9042 (cassandra)'
    );
    formatEndpoint({ipv4: '150.151.152.153', serviceName: 'cassandra'}).should.equal(
      '150.151.152.153 (cassandra)'
    );
  });

  it('should not show empty service name', () => {
    formatEndpoint({ipv4: '150.151.152.153', port: 9042, serviceName: ''}).should.equal(
      '150.151.152.153:9042'
    );
    formatEndpoint({ipv4: '150.151.152.153', serviceName: ''}).should.equal(
      '150.151.152.153'
    );
  });

  it('should show service name missing IP', () => {
    formatEndpoint({serviceName: 'rabbit'}).should.equal(
      'rabbit'
    );
  });

  it('should not crash on no data', () => {
    formatEndpoint({}).should.equal('');
  });

  it('should put ipv6 in brackets', () => {
    formatEndpoint({ipv6: '2001:db8::c001', port: 9042, serviceName: 'cassandra'}).should.equal(
      '[2001:db8::c001]:9042 (cassandra)'
    );

    formatEndpoint({ipv6: '2001:db8::c001', port: 9042}).should.equal(
      '[2001:db8::c001]:9042'
    );

    formatEndpoint({ipv6: '2001:db8::c001'}).should.equal(
      '[2001:db8::c001]'
    );
  });
});
