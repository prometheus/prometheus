import React from 'react';
import { Alert } from 'reactstrap';
import Graph from './Graph';
import { QueryParams } from '../types/types';
import { isPresent } from '../utils/func';

export const GraphTabContent = ({
  data,
  stacked,
  lastQueryParams,
}: {
  data: any;
  stacked: boolean;
  lastQueryParams: QueryParams | null;
}) => {
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
  return <Graph data={data} stacked={stacked} queryParams={lastQueryParams} />;
};
