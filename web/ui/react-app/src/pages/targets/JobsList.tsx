import React, { FC } from 'react';
import { FilterData } from './Filter';
import { useFetch } from '../../utils/useFetch';
import { TargetGroup, groupTargets } from './target';
import JobPanel from './JobPanel';
import Error from '../../ErrorAlert';
import Loader from '../../Loader';
import PathPrefixProps from '../../PathPrefixProps';

interface JobsListProps {
  filter: FilterData;
}

const filterByHealth = ({ metadata, targets }: TargetGroup, { showHealthy, showUnhealthy }: FilterData): boolean => {
  const isHealthy = metadata.up === targets.length;
  return (isHealthy && showHealthy) || (!isHealthy && showUnhealthy);
};

const JobsList: FC<JobsListProps & PathPrefixProps> = ({ filter, pathPrefix }) => {
  const { response, error } = useFetch(`${pathPrefix}/api/v1/targets`);

  if (error) {
    return <Error summary="Error fetching targets" message={error.message} />;
  } else if (response && response.status !== 'success') {
    return <Error summary="Error fetching targets" message={response.status} />;
  } else if (response && response.data) {
    const { activeTargets } = response.data;
    const targetGroups = groupTargets(activeTargets);
    return (
      <>
        {Object.keys(targetGroups)
          .filter((scrapeJob: string) => filterByHealth(targetGroups[scrapeJob], filter))
          .map((scrapeJob: string) => {
            const targetGroupProps = {
              scrapeJob,
              targetGroup: targetGroups[scrapeJob],
            };
            return <JobPanel key={scrapeJob} {...targetGroupProps} />;
          })}
      </>
    );
  } else {
    return <Loader />;
  }
};

export default JobsList;
