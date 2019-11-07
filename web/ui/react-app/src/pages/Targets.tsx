import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert } from 'reactstrap';

const Targets: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => (
  <>
    <h2>Targets</h2>
    <Alert color="warning">
      This page is still under construction. Please try it in the <a href={`${pathPrefix}/targets`}>Classic UI</a>.
    </Alert>
  </>
);

export default Targets;
