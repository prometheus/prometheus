import React, { FC } from 'react';
import { FilterData } from './Filter';
import { useFetch } from '../../hooks/useFetch';
import { groupTargets, Target } from './target';
import ScrapePoolPanel from './ScrapePoolPanel';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';

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

const ScrapePoolList: FC<{ filter: FilterData }> = ({ filter }) => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ScrapePoolListProps>(`${pathPrefix}/${API_PATH}/targets?state=active`);
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
