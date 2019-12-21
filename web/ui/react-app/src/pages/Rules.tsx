import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';
import { Alert, Table } from 'reactstrap';
import { useFetch } from '../utils/useFetch';

interface Rule {
  name: string;
  query: string;
  health: string;
  evaluationTime: number;
  lastEvaluation: number;
  type: string;
}

interface EnrichRule {
  name: string;
  file: string;
  rules: Rule[];
}

interface RulesMap {
  groups: EnrichRule[];
}

const Rules: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<RulesMap>(`${pathPrefix}/api/v1/rules`);
  const stats = response && response.data;
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
        {groups.map(g => {
          return (
            <Table size="sm" striped={true}>
              <thead>
                <th>{g.name}</th>
                <th>{g.file}</th>
              </thead>
              <tbody>
                <tr>
                  <td>dfsdf</td>
                  <td>dsfgd</td>
                  <td>fgdfgf</td>
                </tr>
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
