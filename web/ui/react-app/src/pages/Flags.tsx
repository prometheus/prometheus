import React from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { Fetch, FetchState } from '../api/Fetch';
import PathPrefixProps from '../PathPrefixProps';

export interface FlagMap {
  [key: string]: string;
}

export const Flags = ({ error, data: flags }: FetchState<FlagMap>) => {
  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Error fetching flags: {error.message}
      </Alert>
    );
  } else if (flags) {
    return (
      <Table bordered={true} size="sm" striped={true}>
        <tbody>
          {Object.keys(flags).map(key => {
            return (
              <tr key={key}>
                <th>{key}</th>
                <td>{flags[key]}</td>
              </tr>
            );
          })}
        </tbody>
      </Table>
    );
  }
  return <FontAwesomeIcon icon={faSpinner} spin />;
}

export default ({ pathPrefix = '' }: RouteComponentProps & PathPrefixProps) => {
  return <Fetch url={`${pathPrefix}/api/v1/status/flags`}>{Flags}</Fetch>
}
