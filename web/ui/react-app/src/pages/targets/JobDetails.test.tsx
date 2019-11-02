import React from 'react';
import { shallow } from 'enzyme';
import JobDetails, { columns } from './JobDetails';
import { Table } from 'reactstrap';
import { targetGroups } from './__testdata__/testdata';
import { Target, TargetGroup } from './target';
import EndpointLink from './EndpointLink';
import StateIndicator from './StateIndicator';
import SeriesLabels from './SeriesLabels';

describe('JobDetails', () => {
  const targetGroup: TargetGroup = targetGroups.blackbox;
  const jobDetails = shallow(<JobDetails targetGroup={targetGroup} />);

  it('renders a table', () => {
    const table = jobDetails.find(Table);
    const headers = table.find('th');
    expect(table).toHaveLength(1);
    expect(headers).toHaveLength(6);
    columns.forEach(col => {
      expect(headers.contains(col));
    });
  });

  describe('for each target', () => {
    targetGroup.targets.forEach(({ discoveredLabels, labels, scrapeUrl, lastError, health }: Target, idx: number) => {
      const row = jobDetails.find('tr').at(idx + 1);

      it('renders an EndpointLink with the scrapeUrl', () => {
        const link = row.find(EndpointLink);
        expect(link).toHaveLength(1);
        expect(link.prop('endpoint')).toEqual(scrapeUrl);
      });

      it('renders a StateIndicator for health', () => {
        const td = row.find('td').filterWhere(elem => Boolean(elem.hasClass('state')));
        const stateIndicator = td.find(StateIndicator);
        expect(stateIndicator).toHaveLength(1);
        expect(stateIndicator.prop('health')).toEqual(health);
      });

      it('renders series labels', () => {
        const seriesLabels = row.find(SeriesLabels);
        expect(seriesLabels).toHaveLength(1);
        expect(seriesLabels.prop('discoveredLabels')).toEqual(discoveredLabels);
        expect(seriesLabels.prop('labels')).toEqual(labels);
      });

      it('renders last scrape time', () => {
        const lastScrapeCell = row.find('td').filterWhere(elem => Boolean(elem.hasClass('last-scrape')));
        expect(lastScrapeCell).toHaveLength(1);
      });

      it('renders last scrape duration', () => {
        const lastScrapeCell = row.find('td').filterWhere(elem => Boolean(elem.hasClass('scrape-duration')));
        expect(lastScrapeCell).toHaveLength(1);
      });

      it('renders a StateIndicator for Errors', () => {
        const td = row.find('td').filterWhere(elem => Boolean(elem.hasClass('errors')));
        const stateIndicator = td.find(StateIndicator);
        expect(stateIndicator).toHaveLength(lastError ? 1 : 0);
        if (lastError) {
          expect(stateIndicator.prop('health')).toEqual(health);
          expect(stateIndicator.prop('message')).toEqual(lastError);
        }
      });
    });
  });
});
