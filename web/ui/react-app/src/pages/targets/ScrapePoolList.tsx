import React, { FC } from 'react';
import { FilterData } from './Filter';
import { useFetch } from '../../utils/useFetch';
import { ScrapePool, groupTargets, Target } from './target';
import ScrapePoolPanel from './ScrapePoolPanel';
import PathPrefixProps from '../../PathPrefixProps';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { Alert } from 'reactstrap';

interface TargetsResponse {
  activeTargets: Target[];
  droppedTargets: Target[];
}

interface ScrapePoolListProps {
  filter: FilterData;
}

const filterByHealth = ({ upCount, targets }: ScrapePool, { showHealthy, showUnhealthy }: FilterData): boolean => {
  const isHealthy = upCount === targets.length;
  return (isHealthy && showHealthy) || (!isHealthy && showUnhealthy);
};

const ScrapePoolList: FC<ScrapePoolListProps & PathPrefixProps> = ({ filter, pathPrefix }) => {
  const { response, error } = useFetch<TargetsResponse>(`${pathPrefix}/api/v1/targets?state=active`);

  if (error) {
    return (
      <Alert color="danger">
        <strong>Error fetching targets:</strong> {error.message}
      </Alert>
    );
  } else if (response && response.status !== 'success' && response.status !== 'start fetching') {
    return (
      <Alert color="danger">
        <strong>Error fetching targets:</strong> {response.status}
      </Alert>
    );
  } else if (response && response.data) {
    const { activeTargets } = response.data;
    const targetGroups = groupTargets(activeTargets);
    return (
      <>
        {Object.keys(targetGroups)
          .filter((scrapePool: string) => filterByHealth(targetGroups[scrapePool], filter))
          .map((scrapePool: string) => {
            const targetGroupProps = {
              scrapePool,
              targetGroup: targetGroups[scrapePool],
            };
            return <ScrapePoolPanel key={scrapePool} {...targetGroupProps} />;
          })}
      </>
    );
  }
  return <FontAwesomeIcon icon={faSpinner} spin />;
};

export default ScrapePoolList;
