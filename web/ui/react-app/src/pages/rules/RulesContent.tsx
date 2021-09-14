import React, { FC } from 'react';
import { APIResponse } from '../../hooks/useFetch';
import { Alert, Table, Badge } from 'reactstrap';
import { Link } from 'react-router-dom';
import { formatRelative, createExpressionLink, humanizeDuration, formatDuration } from '../../utils';
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

const GraphExpressionLink: FC<{ expr: string; text: string; title: string }> = (props) => {
  return (
    <>
      <strong>{props.title}:</strong>
      <Link className="ml-4" to={createExpressionLink(props.expr)}>
        {props.text}
      </Link>
      <br />
    </>
  );
};

export const RulesContent: FC<RulesContentProps> = ({ response }) => {
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
                      <h4 id={g.name} className="text-break">
                        {g.name}
                      </h4>
                    </a>
                  </td>
                  <td>
                    <h4>{formatRelative(g.lastEvaluation, now())}</h4>
                  </td>
                  <td>
                    <h4>{humanizeDuration(parseFloat(g.evaluationTime) * 1000)}</h4>
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
                      <td className="rule-cell">
                        {r.alerts ? (
                          <GraphExpressionLink title="alert" text={r.name} expr={`ALERTS{alertname="${r.name}"}`} />
                        ) : (
                          <GraphExpressionLink title="record" text={r.name} expr={r.name} />
                        )}
                        <GraphExpressionLink title="expr" text={r.query} expr={r.query} />
                        {r.duration > 0 && (
                          <div>
                            <strong>for:</strong> {formatDuration(r.duration * 1000)}
                          </div>
                        )}
                        {r.labels && Object.keys(r.labels).length > 0 && (
                          <div>
                            <strong>labels:</strong>
                            {Object.entries(r.labels).map(([key, value]) => (
                              <div className="ml-4" key={key}>
                                {key}: {value}
                              </div>
                            ))}
                          </div>
                        )}
                        {r.alerts && r.annotations && Object.keys(r.annotations).length > 0 && (
                          <div>
                            <strong>annotations:</strong>
                            {Object.entries(r.annotations).map(([key, value]) => (
                              <div className="ml-4" key={key}>
                                {key}: {value}
                              </div>
                            ))}
                          </div>
                        )}
                      </td>
                      <td>
                        <Badge color={getBadgeColor(r.health)}>{r.health.toUpperCase()}</Badge>
                      </td>
                      <td>{r.lastError ? <Alert color="danger">{r.lastError}</Alert> : null}</td>
                      <td>{formatRelative(r.lastEvaluation, now())}</td>
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
