import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';

const Rules: FC<RouteComponentProps & PathPrefixProps> = () => <div>Rules page</div>;

export default Rules;
