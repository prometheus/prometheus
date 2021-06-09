import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch, useFetchReady } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import AlertsContent, { RuleStatus, AlertsProps } from './AlertContents';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { checkReady } from '../../utils';
import Starting from '../starting/Starting';

const AlertsWithStatusIndicator = withStatusIndicator(AlertsContent);

const Alerts: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<AlertsProps>(`${pathPrefix}/${API_PATH}/rules?type=alert`);

  if (!checkReady(useFetchReady(pathPrefix))) {
    return <Starting />;
  }

  const ruleStatsCount: RuleStatus<number> = {
    inactive: 0,
    pending: 0,
    firing: 0,
  };

  if (response.data && response.data.groups) {
    response.data.groups.forEach(el => el.rules.forEach(r => ruleStatsCount[r.state]++));
  }

  return <AlertsWithStatusIndicator {...response.data} statsCount={ruleStatsCount} error={error} isLoading={isLoading} />;
};

export default Alerts;
