import React, { FC, Fragment } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Table } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { useFetch } from '../utils/useFetch';
import PathPrefixProps from '../PathPrefixProps';

export interface Stats {
  name: string;
  value: number;
}

export interface TSDBMap {
  seriesCountByMetricName: Array<Stats>;
  labelValueCountByLabelName: Array<Stats>;
  memoryInBytesByLabelName: Array<Stats>;
  seriesCountByLabelValuePair: Array<Stats>;
}

const paddingStyle = {
  padding: '10px',
};

function createTable(title: string, unit: string, stats: Array<Stats>) {
  return (
    <div style={paddingStyle}>
      <h3>{title}</h3>
      <Table bordered={true} size="sm" striped={true}>
        <thead>
          <tr>
            <th>Name</th>
            <th>{unit}</th>
          </tr>
        </thead>
        <tbody>
          {stats.map((element: Stats, i: number) => {
            return (
              <Fragment key={i}>
                <tr>
                  <td>{element.name}</td>
                  <td>{element.value}</td>
                </tr>
              </Fragment>
            );
          })}
        </tbody>
      </Table>
    </div>
  );
}

const TSDBStatus: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<TSDBMap>(`${pathPrefix}/api/v1/status/tsdb`);
  const headStats = () => {
    const stats = response && response.data;
    if (error) {
      return (
        <Alert color="danger">
          <strong>Error:</strong> Error fetching TSDB Status: {error.message}
        </Alert>
      );
    } else if (stats) {
      return (
        <div>
          <div style={paddingStyle}>
            <h3>Head Cardinality Stats</h3>
          </div>
          {createTable('Top 10 label names with value count', 'Count', stats.labelValueCountByLabelName)}
          {createTable('Top 10 series count by metric names', 'Count', stats.seriesCountByMetricName)}
          {createTable('Top 10 label names with high memory usage', 'Bytes', stats.memoryInBytesByLabelName)}
          {createTable('Top 10 series count by label value pairs', 'Count', stats.seriesCountByLabelValuePair)}
        </div>
      );
    }
    return <FontAwesomeIcon icon={faSpinner} spin />;
  };

  return (
    <div>
      <h2>TSDB Status</h2>
      {headStats()}
    </div>
  );
};

export default TSDBStatus;
