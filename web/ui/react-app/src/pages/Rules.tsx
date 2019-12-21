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
}

interface EnrichRule {
  name: string;
  file: string;
  rules: Rule[];
  evaluationTime: string;
}

interface RulesMap {
  groups: EnrichRule[];
}

const Rules: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<RulesMap>(`${pathPrefix}/api/v1/rules`);
  const getMaximumTimeEvaluation = (rules: Rule[]) => {
    let max = -1;
    for (const rule of rules) {
      max = parseFloat(rule.lastEvaluation) > max ? parseFloat(rule.lastEvaluation) : max;
    }
    return max;
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
            <Table size="sm" striped={true}>
              <thead>
                <tr>
                  <td colSpan={3}>
                    <h2>{g.name}</h2>
                  </td>
                  <td>
                    <h2>{getMaximumTimeEvaluation(g.rules)}</h2>
                  </td>
                  <td>
                    <h2>{g.evaluationTime}</h2>
                  </td>
                </tr>
              </thead>
              <tbody>
                <tr></tr>
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
