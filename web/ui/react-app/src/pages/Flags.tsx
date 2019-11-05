import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { Fetch, FetchState } from '../api/Fetch';
import PathPrefixProps from '../PathPrefixProps';
import { StatusIndicator } from '../StatusIndicator';

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

const Flags: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix = '' }) => {
  return (
    <Fetch url={`${pathPrefix}/api/v1/status/flags`}>
      {({ error, data }: FetchState<FlagMap>) => (
        <StatusIndicator error={error && `Error fetching flags: ${error.message}`} hasData={Boolean(data)}>
          <FlagsContent data={data} />
        </StatusIndicator>
      )}
    </Fetch>
  );
};

export default Flags;
