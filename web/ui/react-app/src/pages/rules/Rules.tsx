import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesMap, RulesContent } from './RulesContent';
import { usePathPrefix } from '../../contexts/PathContexts';
import { APIPATH } from '../../constants/constants';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<RulesMap>(`${pathPrefix}/${APIPATH}/rules`);

  return <RulesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default Rules;
