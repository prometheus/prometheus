import React, { FC, Fragment } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Table, Badge } from 'reactstrap';
import { formatRelative, humanizeDuration, isPresent } from '../../utils';
import { now } from 'moment';
import { RulePanel } from './RulePanel';
import { BaseRule } from '../../types/types';

export interface RuleGroup<T> {
  name: string;
  file: string;
  rules: T[];
  interval: number;
  evaluationTime: string;
  lastEvaluation: string;
}

export interface RulesGroups {
  groups: RuleGroup<BaseRule>[];
}

export const badgeColorMap: Record<'ok' | 'err' | 'unknown', 'success' | 'danger' | 'warning'> = {
  ok: 'success',
  err: 'danger',
  unknown: 'warning',
};

export const RulesContent: FC<RouteComponentProps & RulesGroups> = ({ groups }) => {
  return (
    <>
      <h2>Rules</h2>
      <Table bordered>
        {groups.map(group => {
          const { name: groupName, lastEvaluation, evaluationTime } = group;
          return (
            <Fragment key={groupName}>
              <thead>
                <tr>
                  <td colSpan={3}>
                    <a href={`#${groupName}`}>
                      <h2 id={groupName}>{groupName}</h2>
                    </a>
                  </td>
                  <td>
                    <h2>{formatRelative(lastEvaluation, now())} ago</h2>
                  </td>
                  <td>
                    <h2>{humanizeDuration(parseFloat(evaluationTime) * 1000)}</h2>
                  </td>
                </tr>
              </thead>
              <tbody>
                <tr className="font-weight-bold">
                  <td>Rule</td>
                  <td>State</td>
                  <td>Error</td>
                  <td>Last Evaluation</td>
                  <td>Evaluation Time</td>
                </tr>
                {group.rules.map((rule, i) => {
                  return (
                    <tr key={i}>
                      <RulePanel tag="td" rule={rule} />
                      <td style={{ textAlign: 'center', verticalAlign: 'middle' }}>
                        <Badge className="p-2 px-4 text-uppercase" color={badgeColorMap[rule.health]}>
                          {rule.health}
                        </Badge>
                      </td>
                      <td>{isPresent(rule.lastError) && <Alert color="danger">{rule.lastError}</Alert>}</td>
                      <td>{formatRelative(rule.lastEvaluation, now())} ago</td>
                      <td>{humanizeDuration(parseFloat(rule.evaluationTime) * 1000)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </Fragment>
          );
        })}
      </Table>
    </>
  );
};
