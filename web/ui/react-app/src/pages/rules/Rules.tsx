import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch, useFetchReady } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RulesMap, RulesContent } from './RulesContent';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { checkReady } from '../../utils';
import Starting from '../starting/Starting';

const RulesWithStatusIndicator = withStatusIndicator(RulesContent);

const Rules: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<RulesMap>(`${pathPrefix}/${API_PATH}/rules`);

  if (!checkReady(useFetchReady(pathPrefix))) {
    return <Starting />;
  }

  return <RulesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default Rules;
