import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';

const Targets: FC<RouteComponentProps & PathPrefixProps> = () => <div>Targets page</div>;

export default Targets;
