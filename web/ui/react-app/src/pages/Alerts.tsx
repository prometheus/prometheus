import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert } from 'reactstrap';

const Alerts: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => (
  <>
    <h2>Alerts</h2>
    <Alert color="warning">
      This page is still under construction. Please try it in the <a href={`${pathPrefix}/alerts`}>Classic UI</a>.
    </Alert>
  </>
);

export default Alerts;
