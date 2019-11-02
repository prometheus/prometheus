import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import Filter from './Filter';
import JobsList from './JobsList';
import PathPrefixProps from '../../PathPrefixProps';

const Targets: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const [filter, setFilter] = useState({ showHealthy: true, showUnhealthy: true });
  const filterProps = { filter, setFilter };
  const jobsListProps = { filter, pathPrefix };

  return (
    <>
      <h1>Targets</h1>
      <Filter {...filterProps} />
      <JobsList {...jobsListProps} />
    </>
  );
};

export default Targets;
