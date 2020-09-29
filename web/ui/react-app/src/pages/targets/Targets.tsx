import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import Filter from './Filter';
import ScrapePoolList from './ScrapePoolList';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import { usePathPrefix } from '../../contexts/PathContexts';
import { APIPATH } from '../../constants/constants';

const Targets: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const [filter, setFilter] = useLocalStorage('targets-page-filter', { showHealthy: true, showUnhealthy: true });
  const filterProps = { filter, setFilter };
  const scrapePoolListProps = { filter, pathPrefix, APIPATH };

  return (
    <>
      <h2>Targets</h2>
      <Filter {...filterProps} />
      <ScrapePoolList {...scrapePoolListProps} />
    </>
  );
};

export default Targets;
