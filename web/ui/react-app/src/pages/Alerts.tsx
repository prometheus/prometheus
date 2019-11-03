import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';

const Alerts: FC<RouteComponentProps & PathPrefixProps> = props => <div>Alerts page</div>;

export default Alerts;
