import metricToSeriesName from './MetricFormat';

describe('metricToSeriesName', () => {
  it('returns "{}" if labels is empty', () => {
    const labels = {};
    expect(metricToSeriesName(labels)).toEqual('{}');
  });
  it('returns "metric_name{}" if labels only contains __name__', () => {
    const labels = { __name__: 'metric_name' };
    expect(metricToSeriesName(labels)).toEqual('metric_name{}');
  });
  it('returns "{label1=value_1, ..., labeln=value_n} if there are many labels and no name', () => {
    const labels = { label1: 'value_1', label2: 'value_2', label3: 'value_3' };
    expect(metricToSeriesName(labels)).toEqual('{label1="value_1", label2="value_2", label3="value_3"}');
  });
  it('returns "metric_name{label1=value_1, ... ,labeln=value_n}" if there are many labels and a name', () => {
    const labels = {
      __name__: 'metric_name',
      label1: 'value_1',
      label2: 'value_2',
      label3: 'value_3',
    };
    expect(metricToSeriesName(labels)).toEqual('metric_name{label1="value_1", label2="value_2", label3="value_3"}');
  });
});
