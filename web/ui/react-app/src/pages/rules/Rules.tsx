import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesMap, RulesContent } from './RulesContent';
import useGlobalVars from '../../hooks/useGlobalVars';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC<RouteComponentProps> = () => {
  const { pathPrefix } = useGlobalVars();
  const { response, error, isLoading } = useFetch<RulesMap>(`${pathPrefix}/api/v1/rules`);

  return <RulesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default Rules;
