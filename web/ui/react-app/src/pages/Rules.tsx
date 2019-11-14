import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert } from 'reactstrap';

const Rules: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => (
  <>
    <h2>Rules</h2>
    <Alert color="warning">
      This page is still under construction. Please try it in the <a href={`${pathPrefix}/rules`}>Classic UI</a>.
    </Alert>
  </>
);

export default Rules;
