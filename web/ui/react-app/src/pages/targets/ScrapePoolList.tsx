import React, { FC } from 'react';
import Filter, { FilterData } from './Filter';
import { useFetch } from '../../hooks/useFetch';
import { groupTargets, Target } from './target';
import ScrapePoolPanel from './ScrapePoolPanel';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { useLocalStorage } from '../../hooks/useLocalStorage';

interface ScrapePoolListProps {
  activeTargets: Target[];
}

export const ScrapePoolContent: FC<ScrapePoolListProps> = ({ activeTargets }) => {
  const targetGroups = groupTargets(activeTargets);
  const initialState: FilterData = {
    showHealthy: true,
    showUnhealthy: true,
    expanded: Object.keys(targetGroups).reduce(
      (acc: { [scrapePool: string]: boolean }, scrapePool: string) => ({
        ...acc,
        [scrapePool]: true,
      }),
      {}
    ),
  };
  const [filter, setFilter] = useLocalStorage('targets-page-filter-v2', initialState);
  const { showHealthy, showUnhealthy, expanded } = filter;
  return (
    <>
      <Filter filter={filter} setFilter={setFilter} />
      {Object.keys(targetGroups)
        .filter(scrapePool => {
          const targetGroup = targetGroups[scrapePool];
          const isHealthy = targetGroup.upCount === targetGroup.targets.length;
          return (isHealthy && showHealthy) || (!isHealthy && showUnhealthy);
        })
        .map<JSX.Element>(scrapePool => (
          <ScrapePoolPanel
            key={scrapePool}
            scrapePool={scrapePool}
            targetGroup={targetGroups[scrapePool]}
            expanded={expanded[scrapePool]}
            toggleExpanded={(): void =>
              setFilter({ ...filter, expanded: { ...expanded, [scrapePool]: !expanded[scrapePool] } })
            }
          />
        ))}
    </>
  );
};
ScrapePoolContent.displayName = 'ScrapePoolContent';

const ScrapePoolListWithStatusIndicator = withStatusIndicator(ScrapePoolContent);

const ScrapePoolList: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ScrapePoolListProps>(`${pathPrefix}/${API_PATH}/targets?state=active`);
  const { status: responseStatus } = response;
  const badResponse = responseStatus !== 'success' && responseStatus !== 'start fetching';
  return (
    <ScrapePoolListWithStatusIndicator
      {...response.data}
      error={badResponse ? new Error(responseStatus) : error}
      isLoading={isLoading}
      componentTitle="Targets information"
    />
  );
};

export default ScrapePoolList;
