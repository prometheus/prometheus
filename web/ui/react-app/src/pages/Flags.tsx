import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { useFetch } from '../utils/useFetch';
import PathPrefixProps from '../PathPrefixProps';
import Loader from '../Loader';
import ErrorAlert from '../ErrorAlert';

export interface FlagMap {
  [key: string]: string;
}

const Flags: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch(`${pathPrefix}/api/v1/status/flags`);

  const body = () => {
    const flags: FlagMap = response && response.data;
    if (error) {
      return <ErrorAlert summary="Error fetching flags" message={error.message} />;
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
    return <Loader />;
  };

  return (
    <>
      <h2>Command-Line Flags</h2>
      {body()}
    </>
  );
};

export default Flags;
