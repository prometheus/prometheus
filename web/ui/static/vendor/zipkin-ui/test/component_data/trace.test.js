import {toContextualLogsUrl} from '../../js/component_data/trace';

describe('toContextualLogsUrl', () => {
  it('replaces token in logsUrl when set', () => {
    const kibanaLogsUrl = 'http://company.com/kibana/#/discover?_a=(query:(query_string:(query:\'{traceId}\')))';
    const traceId = '86bad84b319c8379';
    toContextualLogsUrl(kibanaLogsUrl, traceId)
      .should.equal(kibanaLogsUrl.replace('{traceId}', traceId));
  });

  it('returns logsUrl when not set', () => {
    const kibanaLogsUrl = undefined;
    const traceId = '86bad84b319c8379';
    (typeof toContextualLogsUrl(kibanaLogsUrl, traceId)).should.equal('undefined');
  });

  it('returns the same url when token not present', () => {
    const kibanaLogsUrl = 'http://mylogqueryservice.com/';
    const traceId = '86bad84b319c8379';
    toContextualLogsUrl(kibanaLogsUrl, traceId).should.equal(kibanaLogsUrl);
  });
});
