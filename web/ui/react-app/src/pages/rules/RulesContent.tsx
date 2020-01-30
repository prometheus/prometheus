import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { APIResponse } from '../../hooks/useFetch';
import { Alert, Table, Badge } from 'reactstrap';
import { Link } from '@reach/router';
import { formatRelative, createExpressionLink, humanizeDuration } from '../../utils';
import { Rule } from '../../types/types';
import { now } from 'moment';

interface RulesContentProps {
  response: APIResponse<RulesMap>;
}

interface RuleGroup {
  name: string;
  file: string;
  rules: Rule[];
  evaluationTime: string;
  lastEvaluation: string;
}

export interface RulesMap {
  groups: RuleGroup[];
}

const GraphExpressionLink: FC<{ expr: string; title: string }> = props => {
  return (
    <>
      <strong>{props.title}:</strong>
      <Link className="ml-4" to={createExpressionLink(props.expr)}>
        {props.expr}
      </Link>
      <br />
    </>
  );
};

export const RulesContent: FC<RouteComponentProps & RulesContentProps> = ({ response }) => {
  const getBadgeColor = (state: string) => {
    switch (state) {
      case 'ok':
        return 'success';

      case 'err':
        return 'danger';

      case 'unknown':
        return 'warning';
    }
  };

  if (response.data) {
    const groups: RuleGroup[] = response.data.groups;
    return (
      <>
        <h2>Rules</h2>
        {groups.map((g, i) => {
          return (
            <Table bordered key={i}>
              <thead>
                <tr>
                  <td colSpan={3}>
                    <a href={'#' + g.name}>
                      <h2 id={g.name}>{g.name}</h2>
                    </a>
                  </td>
                  <td>
                    <h2>{formatRelative(g.lastEvaluation, now())} ago</h2>
                  </td>
                  <td>
                    <h2>{humanizeDuration(parseFloat(g.evaluationTime) * 1000)}</h2>
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
                {g.rules.map((r, i) => {
                  return (
                    <tr key={i}>
                      {r.alerts ? (
                        <td style={{ backgroundColor: '#F5F5F5' }} className="rule_cell">
                          <GraphExpressionLink title="alert" expr={r.name} />
                          <GraphExpressionLink title="expr" expr={r.query} />
                          <div>
                            <strong>labels:</strong>
                            {Object.entries(r.labels).map(([key, value]) => (
                              <div className="ml-4" key={key}>
                                {key}: {value}
                              </div>
                            ))}
                          </div>
                          <div>
                            <strong>annotations:</strong>
                            {Object.entries(r.annotations).map(([key, value]) => (
                              <div className="ml-4" key={key}>
                                {key}: {value}
                              </div>
                            ))}
                          </div>
                        </td>
                      ) : (
                        <td style={{ backgroundColor: '#F5F5F5' }}>
                          <GraphExpressionLink title="record" expr={r.name} />
                          <GraphExpressionLink title="expr" expr={r.query} />
                        </td>
                      )}
                      <td>
                        <Badge color={getBadgeColor(r.health)}>{r.health.toUpperCase()}</Badge>
                      </td>
                      <td>{r.lastError ? <Alert color="danger">{r.lastError}</Alert> : null}</td>
                      <td>{formatRelative(r.lastEvaluation, now())} ago</td>
                      <td>{humanizeDuration(parseFloat(r.evaluationTime) * 1000)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
          );
        })}
      </>
    );
  }

  return null;
};
