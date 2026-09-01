import React from 'react';
import { render, screen } from '@testing-library/react';
import TSDBStatus, { TSDBMap } from './TSDBStatus';
import { PathPrefixContext } from '../../contexts/PathPrefixContext';

const fakeTSDBStatusResponse: { status: string; data: TSDBMap } = {
  status: 'success',
  data: {
    headStats: {
      numSeries: 508,
      numLabelPairs: 1234,
      chunkCount: 937,
      minTime: 1591516800000,
      maxTime: 1598896800143,
    },
    labelValueCountByLabelName: [{ name: '__name__', value: 5 }],
    seriesCountByMetricName: [
      { name: 'scrape_duration_seconds', value: 1 },
      { name: 'scrape_samples_scraped', value: 1 },
    ],
    memoryInBytesByLabelName: [{ name: '__name__', value: 103 }],
    seriesCountByLabelValuePair: [{ name: 'instance=localhost:9100', value: 5 }],
  },
};

const emptyCardinality = {
  labelValueCountByLabelName: [],
  seriesCountByMetricName: [],
  memoryInBytesByLabelName: [],
  seriesCountByLabelValuePair: [],
};

const renderStatus = () =>
  render(
    <PathPrefixContext.Provider value="/path/prefix">
      <TSDBStatus />
    </PathPrefixContext.Provider>
  );

const tableCells = (table: HTMLElement): string[] =>
  Array.from(table.querySelectorAll('tbody td')).map((cell) => cell.textContent || '');

describe('TSDBStatus', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
  });

  it('fetches and renders head and cardinality statistics', async () => {
    fetchMock.mockResponse(JSON.stringify(fakeTSDBStatusResponse));

    renderStatus();

    expect(await screen.findByRole('heading', { name: 'TSDB Status' })).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledWith('/path/prefix/api/v1/status/tsdb', {
      cache: 'no-store',
      credentials: 'same-origin',
    });

    const tables = screen.getAllByRole('table');
    expect(tableCells(tables[0])).toEqual([
      '508',
      '937',
      '1234',
      '2020-06-07T08:00:00.000Z (1591516800000)',
      '2020-08-31T18:00:00.143Z (1598896800143)',
    ]);
    expect(tableCells(tables[1])).toEqual(['__name__', '5']);
    expect(tableCells(tables[2])).toEqual(['scrape_duration_seconds', '1', 'scrape_samples_scraped', '1']);
    expect(tableCells(tables[3])).toEqual(['__name__', '103']);
    expect(tableCells(tables[4])).toEqual(['instance=localhost:9100', '5']);
  });

  it.each([
    {
      name: 'no datapoints',
      numSeries: 0,
      expectedMin: 'No datapoints yet',
      expectedMax: 'No datapoints yet',
    },
    {
      name: 'invalid timestamps with existing series',
      numSeries: 1,
      expectedMin: 'Error parsing time (9223372036854776000)',
      expectedMax: 'Error parsing time (-9223372036854776000)',
    },
  ])('renders $name', async ({ numSeries, expectedMin, expectedMax }) => {
    fetchMock.mockResponse(
      JSON.stringify({
        status: 'success',
        data: {
          headStats: {
            numSeries,
            numLabelPairs: 0,
            chunkCount: 0,
            minTime: 9223372036854776000,
            maxTime: -9223372036854776000,
          },
          ...emptyCardinality,
        },
      })
    );

    renderStatus();
    await screen.findByRole('heading', { name: 'TSDB Status' });

    expect(tableCells(screen.getAllByRole('table')[0])).toEqual([numSeries.toString(), '0', '0', expectedMin, expectedMax]);
  });
});
