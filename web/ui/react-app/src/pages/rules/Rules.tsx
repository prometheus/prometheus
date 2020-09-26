import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesMap, RulesContent } from './RulesContent';
import { usePathPrefix, useAPIPath } from '../../contexts/PathContexts';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const apiPath = useAPIPath();
  const { response, error, isLoading } = useFetch<RulesMap>(`${pathPrefix}/${apiPath}/rules`);

  return <RulesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default Rules;
