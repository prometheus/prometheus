import * as React from 'react';
import { shallow } from 'enzyme';
import QueryStatsView from './QueryStatsView';

describe('QueryStatsView', () => {
  it('renders props as query stats', () => {
    const queryStatsProps = {
      loadTime: 100,
      resolution: 5,
      resultSeries: 10000,
    };
    const queryStatsView = shallow(<QueryStatsView {...queryStatsProps} />);
    expect(queryStatsView.prop('className')).toEqual('query-stats');
    expect(queryStatsView.children().prop('className')).toEqual('float-right');
    expect(queryStatsView.children().text()).toEqual('Load time: 100ms   Resolution: 5s   Result series: 10000');
  });
});
