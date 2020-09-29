import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { useFetch } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathContexts';
import { APIPATH } from '../../constants/constants'

interface FlagMap {
  [key: string]: string;
}

interface FlagsProps {
  data?: FlagMap;
}

export const FlagsContent: FC<FlagsProps> = ({ data = {} }) => {
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
const FlagsWithStatusIndicator = withStatusIndicator(FlagsContent);

FlagsContent.displayName = 'Flags';

const Flags: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<FlagMap>(`${pathPrefix}/${APIPATH}/status/flags`);
  return <FlagsWithStatusIndicator data={response.data} error={error} isLoading={isLoading} />;
};

export default Flags;
