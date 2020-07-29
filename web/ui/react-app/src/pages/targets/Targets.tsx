import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import Filter from './Filter';
import ScrapePoolList from './ScrapePoolList';
import PathPrefixProps from '../../types/PathPrefixProps';
import { useLocalStorage } from '../../hooks/useLocalStorage';

const Targets: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const [filter, setFilter] = useLocalStorage('targets-page-filter', { showHealthy: true, showUnhealthy: true });
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
