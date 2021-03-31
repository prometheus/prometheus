import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import ScrapePoolList from './ScrapePoolList';

const Targets: FC<RouteComponentProps> = () => {
  return (
    <>
      <h2>Targets</h2>
      <ScrapePoolList />
    </>
  );
};

export default Targets;
