import {
  traceSummary,
  getServiceName,
  getTraceErrorType,
  traceSummariesToMustache,
  mkDurationStr,
  totalServiceTime
} from '../../js/component_ui/traceSummary';
import {Constants} from '../../js/component_ui/traceConstants';
import {endpoint, annotation, binaryAnnotation, span} from './traceTestHelpers';

chai.config.truncateThreshold = 0;


const ep1 = endpoint(123, 123, 'service1');
const ep2 = endpoint(456, 456, 'service2');
const ep3 = endpoint(666, 666, 'service2');
const ep4 = endpoint(777, 777, 'service3');
const ep5 = endpoint(888, 888, 'service3');

describe('traceSummary', () => {
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
  const span5Id = '1111';

  const span1 = span(12345, 'methodcall1', span1Id, null, 100, 50, annotations1);
  const span2 = span(12345, 'methodcall2', span2Id, span1Id, 200, 50, annotations2);
  const span3 = span(12345, 'methodcall2', span3Id, span2Id, 300, 50, annotations3);
  const span4 = span(12345, 'methodcall2', span4Id, span3Id, 400, 100, annotations4);
  const span5 = span(12345, 'methodcall4', span5Id, span4Id);

  const trace = [span1, span2, span3, span4];

  it('should return null when no spans exist', () => {
    expect(traceSummary([])).to.equal(null);
  });

  it('should return null when no annotations are present', () => {
    expect(traceSummary([span5])).to.equal(null);
  });

  it('dedupes duplicate endpoints', () => {
    const summary = traceSummary(trace);
    summary.endpoints.should.eql([ep1, ep2, ep3, ep4, ep5]);
  });

  it('calculates timestamp and duration', () => {
    const summary = traceSummary(trace);
    summary.timestamp.should.equal(100);
    summary.duration.should.equal(400);
  });

  it('should get total spans count', () => {
    const summary = traceSummary(trace);
    summary.totalSpans.should.equal(trace.length);
  });
});

describe('get service name of a span', () => {
  it('should get service name from server addr', () => {
    const testSpan = {
      binaryAnnotations: [{
        key: Constants.SERVER_ADDR,
        value: '1',
        endpoint: {
          serviceName: 'user-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('user-service');
  });

  it('should get service name from broker addr', () => {
    const testSpan = {
      binaryAnnotations: [{
        key: Constants.MESSAGE_ADDR,
        value: '1',
        endpoint: {
          serviceName: 'kafka'
        }
      }]
    };
    getServiceName(testSpan).should.equal('kafka');
  });

  it('should get service name from some server annotation', () => {
    const testSpan = {
      annotations: [{
        value: Constants.SERVER_RECEIVE_FRAGMENT,
        endpoint: {
          serviceName: 'test-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('test-service');
  });

  it('should get service name from producer annotation', () => {
    const testSpan = {
      annotations: [{
        value: Constants.MESSAGE_SEND,
        endpoint: {
          serviceName: 'test-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('test-service');
  });

  it('should get service name from consumer annotation', () => {
    const testSpan = {
      annotations: [{
        value: Constants.MESSAGE_RECEIVE,
        endpoint: {
          serviceName: 'test-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('test-service');
  });

  it('should get service name from client addr', () => {
    const testSpan = {
      binaryAnnotations: [{
        key: Constants.CLIENT_ADDR,
        value: 'something',
        endpoint: {
          serviceName: 'my-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('my-service');
  });

  it('should get service name from client annotation', () => {
    const testSpan = {
      annotations: [{
        value: Constants.CLIENT_SEND,
        endpoint: {
          serviceName: 'abc-service'
        }
      }]
    };
    getServiceName(testSpan).should.equal('abc-service');
  });

  it('should get service name from local component annotation', () => {
    const testSpan = {
      binaryAnnotations: [{
        key: Constants.LOCAL_COMPONENT,
        value: 'something',
        endpoint: {
          serviceName: 'localservice'
        }
      }]
    };
    getServiceName(testSpan).should.equal('localservice');
  });

  it('should get service name from any binary annotation', () => {
    const testSpan = {
      binaryAnnotations: [{
        key: 'user',
        value: 'grpc-client-example',
        endpoint: {
          serviceName: 'echecklist-localdev'
        }
      }]
    };
    getServiceName(testSpan).should.equal('echecklist-localdev');
  });

  it('should handle no annotations', () => {
    expect(getServiceName({})).to.equal(null);
  });
});

describe('getTraceErrorType', () => {
  const annotationsNoError = [
    annotation(100, Constants.CLIENT_SEND, ep1),
    annotation(150, Constants.CLIENT_RECEIVE, ep1)
  ];

  const annotationsHasError = [
    annotation(100, Constants.CLIENT_SEND, ep1),
    annotation(150, Constants.CLIENT_RECEIVE, ep1),
    annotation(200, Constants.ERROR, ep1)
  ];

  const binaryAnnotationsNoError = [
    binaryAnnotation('key', 'value', ep1)
  ];

  const binaryAnnotationsHasError = [
    binaryAnnotation('key', 'value', ep1),
    binaryAnnotation(Constants.ERROR, 'bad stuff happened', ep1)
  ];

  it('should return none if annotations and binary annotations are null', () => {
    const testSpans = [
      span(12345, 'name', 12345)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('none');
  });

  it('should return none if annotations and binary annotations are null', () => {
    const testSpans = [
      span(12345, 'name', 12345),
      span(12346, 'name', 12346)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('none');
  });

  it('should return none if ann=noError and binAnn=null', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           null, binaryAnnotationsNoError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('none');
  });

  it('should return none if ann=noError and binAnn=null', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsNoError, binaryAnnotationsNoError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('none');
  });

  it('should return none if second span has ann=noError and binAnn=noError', () => {
    const testSpans = [
      span(123456, 'name', 123456),
      span(12345, 'name', 12345, null, null, null,
           annotationsNoError, binaryAnnotationsNoError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('none');
  });

  it('should return critical if ann=null and bin=error', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           null, binaryAnnotationsHasError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('critical');
  });

  it('should return critical if ann=noError and bin=error', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsNoError, binaryAnnotationsHasError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('critical');
  });

  it('should return critical if ann=error and bin=error', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsHasError, binaryAnnotationsHasError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('critical');
  });

  it('should return critical if span1 has ann=error and span2 has binAnn=error', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsHasError, null),
      span(123456, 'name', 123456, null, null, null,
            null, binaryAnnotationsHasError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('critical');
  });

  it('should return transient if ann=error and bin=null', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsHasError, null)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('transient');
  });

  it('should return transient if ann=error and bin=noError', () => {
    const testSpans = [
      span(12345, 'name', 12345, null, null, null,
           annotationsHasError, binaryAnnotationsNoError)
    ];
    expect(getTraceErrorType(testSpans)).to.equal('transient');
  });
});

describe('traceSummariesToMustache', () => {
  const start = 1456447911000000;
  const summary = {
    traceId: 'cafedead',
    timestamp: start,
    duration: 20000,
    spanTimestamps: [{
      name: 'A',
      timestamp: start,
      duration: 10000
    }, {
      name: 'B',
      timestamp: start + 1000,
      duration: 20000
    }, {
      name: 'B',
      timestamp: start + 1000,
      duration: 15000
    }],
    endpoints: [ep1, ep2]
  };

  it('should return empty list for empty list', () => {
    traceSummariesToMustache(null, []).should.eql([]);
  });

  it('should convert duration from micros to millis', () => {
    const model = traceSummariesToMustache(null, [{duration: 3000}]);
    model[0].duration.should.equal(3);
  });

  it('should get service durations', () => {
    const model = traceSummariesToMustache(null, [summary]);
    model[0].serviceDurations.should.eql([{
      name: 'A',
      count: 1,
      max: 10
    }, {
      name: 'B',
      count: 2,
      max: 20
    }]);
  });

  it('should pass on the trace id', () => {
    const model = traceSummariesToMustache('A', [summary]);
    model[0].traceId.should.equal(summary.traceId);
  });

  it('should get service percentage', () => {
    const model = traceSummariesToMustache('A', [summary]);
    model[0].servicePercentage.should.equal(50);
  });

  it('should format start time', () => {
    const model = traceSummariesToMustache(null, [summary], true);
    model[0].startTs.should.equal('02-26-2016T00:51:51.000+0000');
  });

  it('should format duration', () => {
    const model = traceSummariesToMustache(null, [summary]);
    model[0].durationStr.should.equal('20.000ms');
  });

  it('should calculate the width in percent', () => {
    const model = traceSummariesToMustache(null, [summary]);
    model[0].width.should.equal(100);
  });

  it('should pass on timestamp', () => {
    const model = traceSummariesToMustache(null, [summary]);
    model[0].timestamp.should.equal(summary.timestamp);
  });

  it('should get correct totalSpans', () => {
    const spans = [{
      traceId: 'd397ce70f5192a8b',
      name: 'get',
      id: 'd397ce70f5192a8b',
      timestamp: 1457160374149000,
      duration: 2000,
      annotations: [{
        timestamp: 1457160374149000,
        value: 'sr',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }, {
        timestamp: 1457160374151000,
        value: 'ss',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }],
      binaryAnnotations: [{
        key: 'http.path',
        value: '/api/v1/services',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1'}
      }, {
        key: 'srv/finagle.version',
        value: '6.33.0',
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1'}
      }, {
        key: 'sa',
        value: true,
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 9411}
      }, {
        key: 'ca',
        value: true,
        endpoint: {serviceName: 'zipkin-query', ipv4: '127.0.0.1', port: 56828}
      }]
    }];
    const testSummary = traceSummary(spans);
    const model = traceSummariesToMustache(null, [testSummary])[0];
    model.totalSpans.should.equal(1);
  });

  it('should order traces by duration and tie-break using trace id', () => {
    const traceId1 = '9ed44141f679130b';
    const traceId2 = '6ff1c14161f7bde1';
    const traceId3 = '1234561234561234';
    const summary1 = traceSummary([{
      traceId: traceId1,
      name: 'get',
      id: '6ff1c14161f7bde1',
      timestamp: 1457186441657000,
      duration: 4000}]);
    const summary2 = traceSummary([{
      traceId: traceId2,
      name: 'get',
      id: '9ed44141f679130b',
      timestamp: 1457186568026000,
      duration: 4000
    }]);
    const summary3 = traceSummary([{
      traceId: traceId3,
      name: 'get',
      id: '6677567324735',
      timestamp: 1457186568027000,
      duration: 3000
    }]);

    const model = traceSummariesToMustache(null, [summary1, summary2, summary3]);
    model[0].traceId.should.equal(traceId2);
    model[1].traceId.should.equal(traceId1);
    model[2].traceId.should.equal(traceId3);
  });
});

describe('mkDurationStr', () => {
  it('should return empty string on zero duration', () => {
    mkDurationStr(0).should.equal('');
  });

  it('should return empty string on undefined duration', () => {
    mkDurationStr().should.equal('');
  });

  it('should format microseconds', () => {
    mkDurationStr(3).should.equal('3μ');
  });

  it('should format ms', () => {
    mkDurationStr(1500).should.equal('1.500ms');
  });

  it('should format seconds', () => {
    mkDurationStr(2534999).should.equal('2.535s');
  });
});

describe('totalServiceTime', () => {
  const time1 = {name: 'service', timestamp: 1456447911000000, duration: 1000};
  const time2 = {name: 'service', timestamp: 1456447912000000, duration: 2000};
  const time3 = {name: 'service', timestamp: 1456447913000000, duration: 3000};

  it('should return zero on empty input', () => {
    totalServiceTime([]).should.equal(0);
  });

  it('should return duration on single input', () => {
    totalServiceTime([time1]).should.equal(time1.duration);
  });

  it('should sum on multiple inputs', () => {
    totalServiceTime([time1, time2, time3]).should.equal(6000);
  });

  it('shouldnt infinitely recurse when duration is undefined', () => {
    // when json form of span is missing the duration key
    const undefinedDuration = {name: 'zipkin-web', timestamp: time1.timestamp, duration: undefined};
    totalServiceTime([time1, time2, time3, undefinedDuration]).should.equal(6000);
  });
});
