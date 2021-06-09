import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import ScrapePoolList from './ScrapePoolList';

import { useFetchReady } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { checkReady } from '../../utils';
import Starting from '../starting/Starting';

const Targets: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  if (!checkReady(useFetchReady(pathPrefix))) {
    return <Starting />;
  }

  return (
    <>
      <h2>Targets</h2>
      <ScrapePoolList />
    </>
  );
};

export default Targets;
