import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert } from 'reactstrap';

const Services: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => (
  <>
    <h2>Service Discovery</h2>
    <Alert color="warning">
      This page is still under construction. Please try it in the <a href={`${pathPrefix}/service-discovery`}>Classic UI</a>.
    </Alert>
  </>
);

export default Services;
