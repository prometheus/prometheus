import React, { FC } from 'react';
import { FilterData } from './Filter';
import { useFetch } from '../../hooks/useFetch';
import { groupTargets, Target } from './target';
import ScrapePoolPanel from './ScrapePoolPanel';
import PathPrefixProps from '../../types/PathPrefixProps';
import { withStatusIndicator } from '../../components/withStatusIndicator';

interface ScrapePoolListProps {
  filter: FilterData;
  activeTargets: Target[];
}

export const ScrapePoolContent: FC<ScrapePoolListProps> = ({ filter, activeTargets }) => {
  const targetGroups = groupTargets(activeTargets);
  const { showHealthy, showUnhealthy } = filter;
  return (
    <>
      {Object.keys(targetGroups).reduce<JSX.Element[]>((panels, scrapePool) => {
        const targetGroup = targetGroups[scrapePool];
        const isHealthy = targetGroup.upCount === targetGroup.targets.length;
        return (isHealthy && showHealthy) || (!isHealthy && showUnhealthy)
          ? [...panels, <ScrapePoolPanel key={scrapePool} scrapePool={scrapePool} targetGroup={targetGroup} />]
          : panels;
      }, [])}
    </>
  );
};
ScrapePoolContent.displayName = 'ScrapePoolContent';

const ScrapePoolListWithStatusIndicator = withStatusIndicator(ScrapePoolContent);

const ScrapePoolList: FC<{ filter: FilterData } & PathPrefixProps> = ({ pathPrefix, filter }) => {
  const { response, error, isLoading } = useFetch<ScrapePoolListProps>(`${pathPrefix}/api/v1/targets?state=active`);
  const { status: responseStatus } = response;
  const badResponse = responseStatus !== 'success' && responseStatus !== 'start fetching';
  return (
    <ScrapePoolListWithStatusIndicator
      {...response.data}
      filter={filter}
      error={badResponse ? new Error(responseStatus) : error}
      isLoading={isLoading}
      componentTitle="Targets information"
    />
  );
};

export default ScrapePoolList;
