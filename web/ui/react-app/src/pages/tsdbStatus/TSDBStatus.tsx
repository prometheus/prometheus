import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Table } from 'reactstrap';

import { useFetch } from '../../hooks/useFetch';
import PathPrefixProps from '../../types/PathPrefixProps';
import { withStatusIndicator } from '../../components/withStatusIndicator';

interface Stats {
  name: string;
  value: number;
}

export interface TSDBMap {
  seriesCountByMetricName: Stats[];
  labelValueCountByLabelName: Stats[];
  memoryInBytesByLabelName: Stats[];
  seriesCountByLabelValuePair: Stats[];
}

export const TSDBStatusContent: FC<TSDBMap> = ({
  labelValueCountByLabelName,
  seriesCountByMetricName,
  memoryInBytesByLabelName,
  seriesCountByLabelValuePair,
}) => {
  return (
    <div>
      <h2>TSDB Status</h2>
      <h3 className="p-2">Head Cardinality Stats</h3>
      {[
        { title: 'Top 10 label names with value count', stats: labelValueCountByLabelName },
        { title: 'Top 10 series count by metric names', stats: seriesCountByMetricName },
        { title: 'Top 10 label names with high memory usage', unit: 'Bytes', stats: memoryInBytesByLabelName },
        { title: 'Top 10 series count by label value pairs', stats: seriesCountByLabelValuePair },
      ].map(({ title, unit = 'Count', stats }) => {
        return (
          <div className="p-2" key={title}>
            <h3>{title}</h3>
            <Table bordered size="sm" striped>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>{unit}</th>
                </tr>
              </thead>
              <tbody>
                {stats.map(({ name, value }) => {
                  return (
                    <tr key={name}>
                      <td>{name}</td>
                      <td>{value}</td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
          </div>
        );
      })}
    </div>
  );
};
TSDBStatusContent.displayName = 'TSDBStatusContent';

const TSDBStatusContentWithStatusIndicator = withStatusIndicator(TSDBStatusContent);

const TSDBStatus: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error, isLoading } = useFetch<TSDBMap>(`${pathPrefix}/api/v1/status/tsdb`);

  return (
    <TSDBStatusContentWithStatusIndicator
      error={error}
      isLoading={isLoading}
      {...response.data}
      componentTitle="TSDB Status information"
    />
  );
};

export default TSDBStatus;
