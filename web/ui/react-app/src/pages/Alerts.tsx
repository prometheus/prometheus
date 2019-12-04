import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
// import { Alert, TabContent, TabPane } from 'reactstrap';
import { useFetch } from '../utils/useFetch';
import { withStatusIndicator } from '../withStatusIndicator';
import { ButtonGroup, Alert, Collapse, Card } from 'reactstrap';
// import { Button, Collapse } from 'reactstrap';

interface AlertsProps {
  groups?: any[];
}

const byAlertType = (group: any) => group.rules.some((e: any) => e.type === 'alerting');

export const AlertsContent: FC<AlertsProps> = ({ groups = [] }) => {
  const alertGroups = groups.filter(byAlertType);
  console.log(alertGroups);

  return <ButtonGroup>{Object}</ButtonGroup>;
};
const AlertsWithStatusIndicator = withStatusIndicator(AlertsContent);

AlertsContent.displayName = 'Alerts';

const Colap = ({ rule }: any) => {
  const [open, toggle] = useState(false)
  console.log(rule);

  return (
    <>
      <Alert onClick={() => toggle(!open)} color="danger">{rule.name}({`${rule.alerts.length} active`})</Alert>
      <Collapse isOpen={open}>
        <Card>
          <div>name: {rule.name}</div>
          <div>expr: {rule.query}</div>
          <div>
            <div>labels:</div>
            <div className="ml-5">severity: {rule.labels.severity}</div>
          </div>
          <div>
            <div>annotations:</div>
            <div className="ml-5">summary: {rule.annotations.summary}</div>
          </div>
        </Card>
      </Collapse>
    </>
  )
}

const Alerts: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  // const { response, error, isLoading } = useFetch<AlertsProps>(`${pathPrefix}/api/v1/rules`);
  const { response } = useFetch<AlertsProps>(`${pathPrefix}/api/v1/rules?type=alert`);
  response.data && console.log(response.data.groups);
  // return <AlertsWithStatusIndicator groups={response.data && response.data.groups} error={error} isLoading={isLoading} />;
  if (response.data && response.data.groups) {
    return (
      <div>
        {response.data.groups.map(g => {
          return (
            <>
              <div>{g.file} > {g.name}</div>
              {g.rules.map((rule: any) => {
                return <Colap rule={rule} />
              })}
            </>
          )
        })}
      </div>
    );
  }
  return null
};

export default Alerts;
