import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { useFetch } from '../utils/useFetch';
import PathPrefixProps from '../PathPrefixProps';

export interface FlagMap {
  [key: string]: string;
}

const Flags: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch(`${pathPrefix}/api/v1/status/flags`);

  const body = () => {
    const flags: FlagMap = response && response.data;
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
  };

  return (
    <>
      <h2>Command-Line Flags</h2>
      {body()}
    </>
  );
};

export default Flags;
