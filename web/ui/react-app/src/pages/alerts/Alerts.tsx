import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../../PathPrefixProps';
import { useFetch } from '../../utils/useFetch';
import { withStatusIndicator } from '../../withStatusIndicator';
import AlertsContent, { RuleStatus, AlertsProps } from './AlertContents';

const AlertsWithStatusIndicator = withStatusIndicator(AlertsContent);

const Alerts: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  const { response, error, isLoading } = useFetch<AlertsProps>(`${pathPrefix}/api/v1/rules?type=alert`);

  const ruleStatsCount: RuleStatus<number> = {
    inactive: 0,
    pending: 0,
    firing: 0,
  };

  if (response.data && response.data.groups) {
    response.data.groups.forEach(el => el.rules.forEach(r => ruleStatsCount[r.state]++));
  }

  return (
    <AlertsWithStatusIndicator
      statsCount={ruleStatsCount}
      groups={response.data && response.data.groups}
      error={error}
      isLoading={isLoading}
    />
  );
};

export default Alerts;
