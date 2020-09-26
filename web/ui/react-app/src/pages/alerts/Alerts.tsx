import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import AlertsContent, { RuleStatus, AlertsProps } from './AlertContents';
import { usePathPrefix, useAPIPath } from '../../contexts/PathContexts';

const AlertsWithStatusIndicator = withStatusIndicator(AlertsContent);

const Alerts: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const apiPath = useAPIPath();
  const { response, error, isLoading } = useFetch<AlertsProps>(`${pathPrefix}/${apiPath}/rules?type=alert`);

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
