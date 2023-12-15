import { isHeatmapData } from './GraphHeatmapHelpers';

describe('GraphHeatmapHelpers', () => {
  it('isHeatmapData should return false for a vector', () => {
    const data = {
      resultType: 'vector',
      result: [
        {
          metric: {
            __name__: 'my_gauge',
            job: 'target',
          },
          value: [1703091180.683, '6'],
        },
      ],
    };
    expect(isHeatmapData(data)).toBe(false);
  });

  it('isHeatmapData should return false for scalar resultType', () => {
    const data = {
      resultType: 'scalar',
      result: [1703091180.125, '1703091180.125'],
    };
    expect(isHeatmapData(data)).toBe(false);
  });
});
