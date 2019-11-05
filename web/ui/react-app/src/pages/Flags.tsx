import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { FetchState } from '../api/Fetch';
import { fetchWithStatus } from '../api/FetchWithStatus';

export interface FlagMap {
  [key: string]: string;
}

export const FlagsContent: FC<FetchState<FlagMap>> = ({ data = {} }) => {
  return (
    <>
      <h2>Command-Line Flags</h2>
      <Table bordered size="sm" striped>
        <tbody>
          {Object.keys(data).map(key => (
            <tr key={key}>
              <th>{key}</th>
              <td>{data[key]}</td>
            </tr>
          ))}
        </tbody>
      </Table>
    </>
  );
};

FlagsContent.displayName = 'Flags';

export default fetchWithStatus<RouteComponentProps, FlagMap>(FlagsContent, `/api/v1/status/flags`);
