import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import Filter from './Filter';
import ScrapePoolList from './ScrapePoolList';
import PathPrefixProps from '../../PathPrefixProps';

const Targets: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const [filter, setFilter] = useState({ showHealthy: true, showUnhealthy: true });
  const filterProps = { filter, setFilter };
  const scrapePoolListProps = { filter, pathPrefix };

  return (
    <>
      <h2>Targets</h2>
      <Filter {...filterProps} />
      <ScrapePoolList {...scrapePoolListProps} />
    </>
  );
};

export default Targets;
