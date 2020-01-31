import React, { FC } from 'react';
import { Alert } from 'reactstrap';
import Graph from './Graph';
import { QueryParams } from '../../types/types';
import { isPresent } from '../../utils';

interface GraphTabContentProps {
  data: any;
  stacked: boolean;
  useLocalTime: boolean;
  lastQueryParams: QueryParams | null;
}

export const GraphTabContent: FC<GraphTabContentProps> = ({ data, stacked, useLocalTime, lastQueryParams }) => {
  if (!isPresent(data)) {
    return <Alert color="light">No data queried yet</Alert>;
  }
  if (data.result.length === 0) {
    return <Alert color="secondary">Empty query result</Alert>;
  }
  if (data.resultType !== 'matrix') {
    return (
      <Alert color="danger">Query result is of wrong type '{data.resultType}', should be 'matrix' (range vector).</Alert>
    );
  }
  return <Graph data={data} stacked={stacked} useLocalTime={useLocalTime} queryParams={lastQueryParams} />;
};
