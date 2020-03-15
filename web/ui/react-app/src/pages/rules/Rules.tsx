import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../../types/PathPrefixProps';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesGroups, RulesContent } from './RulesContent';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error, isLoading } = useFetch<RulesGroups>(`${pathPrefix}/api/v1/rules`);

  return <RulesWithStatusIndicator {...response.data} error={error} isLoading={isLoading} />;
};

export default Rules;
