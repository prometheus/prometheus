import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert, Table } from 'reactstrap';
import { useFetch } from '../utils/useFetch';

interface Rule {
  name: string;
  query: string;
  health: string;
  evaluationTime: string;
  lastEvaluation: string;
  type: string;
  lastError: string;
}

interface EnrichRule {
  name: string;
  file: string;
  rules: Rule[];
  evaluationTime: string;
  lastEvaluation: string;
}

interface RulesMap {
  groups: EnrichRule[];
}

const Rules: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<RulesMap>(`${pathPrefix}/api/v1/rules`);
  console.warn('response ', response);
  const getMaximumTimeEvaluation = (rules: Rule[]) => {
    let max = -1;
    for (const rule of rules) {
      max = parseFloat(rule.lastEvaluation) > max ? parseFloat(rule.lastEvaluation) : max;
    }
    return max;
  };
  const roundUp = (n: number) => {
    return Math.floor(n * 100) / 100;
  };
  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching Rules: {error.message}
      </Alert>
    );
  } else if (response.data) {
    const groups: EnrichRule[] = response.data.groups;
    return (
      <>
        <h2>Rules</h2>
        {groups.map((g, i) => {
          return (
            <Table size="sm" striped={true} bordered={true} key={i}>
              <thead>
                <tr>
                  <td colSpan={3}>
                    <h2>{g.name}</h2>
                  </td>
                  <td>
                    <h2>{roundUp(parseFloat(g.lastEvaluation))}s ago</h2>
                  </td>
                  <td>
                    <h2>{g.evaluationTime}us</h2>
                  </td>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td className="rules-head">Rule</td>
                  <td className="rules-head">State</td>
                  <td className="rules-head">Error</td>
                  <td className="rules-head">Last Evaluation</td>
                  <td className="rules-head">Evaluation Time</td>
                </tr>
                {groups.map((g, _) => {
                  return g.rules.map((r, i) => {
                    return (
                      <tr key={i}>
                        <td>
                          record: {r.name} <br />
                          expr: {r.query}
                        </td>
                        <td>
                          <Alert style={{ display: 'inline-table' }}>{r.health.toUpperCase()}</Alert>
                        </td>
                        <td>{r.lastError.length !== 0 ? r.lastError : null}</td>
                        <td>{roundUp(parseFloat(r.lastEvaluation))}s ago</td>
                        <td>{r.evaluationTime}us ago</td>
                      </tr>
                    );
                  });
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

export default Rules;
