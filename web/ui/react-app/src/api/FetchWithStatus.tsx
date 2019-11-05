import React, { FC, ComponentType } from 'react';
import { Fetch, FetchState } from '../api/Fetch';
import { StatusIndicator } from '../StatusIndicator';
import PathPrefixProps from '../PathPrefixProps';

export const fetchWithStatus = <T extends {}, DataType extends {}>(
  Component: ComponentType<{ data?: DataType }>,
  url?: string,
  urls?: string[]
): FC<PathPrefixProps & T> => ({ pathPrefix = '' }) => {
  return (
    <Fetch url={url && pathPrefix + url} urls={urls && urls.map(url => pathPrefix + url)}>
      {({ error, data }: FetchState<DataType>) => {
        return (
          <StatusIndicator
            error={error && `Error fetching ${Component.displayName}: ${error.message}`}
            hasData={Boolean(data)}
          >
            <Component data={data} />
          </StatusIndicator>
        );
      }}
    </Fetch>
  );
};
