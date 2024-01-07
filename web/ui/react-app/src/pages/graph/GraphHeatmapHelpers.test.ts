import { DataTableProps } from './DataTable';
import { isHeatmapData } from './GraphHeatmapHelpers';

describe('GraphHeatmapHelpers', () => {
  it('isHeatmapData should return false for scalar and string resultType', () => {
    let data = {
      resultType: 'scalar',
      result: [1703091180.125, '1703091180.125'],
    } as DataTableProps['data'];
    expect(isHeatmapData(data)).toBe(false);

    data = {
      resultType: 'string',
      result: [1704305680.332, '2504'],
    } as DataTableProps['data'];
    expect(isHeatmapData(data)).toBe(false);
  });

  it('isHeatmapData should return false for a vector and matrix if length < 2', () => {
    let data = {
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
    } as DataTableProps['data'];
    expect(isHeatmapData(data)).toBe(false);

    data = {
      resultType: 'matrix',
      result: [
        {
          metric: {},
          values: [[1703091180.683, '6']],
        },
      ],
    } as DataTableProps['data'];
    expect(isHeatmapData(data)).toBe(false);
  });

  it('isHeatmapData should return true for valid heatmap data', () => {
    const data = {
      resultType: 'matrix',
      result: [
        {
          metric: {
            le: '100',
          },
          values: [[1703091180.683, '6']],
        },
        {
          metric: {
            le: '1000',
          },
          values: [[1703091190.683, '6.1']],
        },
      ],
    } as DataTableProps['data'];
    expect(isHeatmapData(data)).toBe(true);
  });
});
