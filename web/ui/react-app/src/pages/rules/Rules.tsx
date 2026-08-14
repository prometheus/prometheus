import React, { FC } from 'react';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesMap, RulesContent } from './RulesContent';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<RulesMap>(`${pathPrefix}/${API_PATH}/rules`);

  return <RulesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default Rules;
