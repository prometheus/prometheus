import React, { FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { APIResponse, useFetch } from '../../hooks/useFetch';
import { API_PATH } from '../../constants/constants';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { RemoteWriteRes, RemoteWritesContent } from './RemoteWritesContent';

const RemoteWritesWithStatusIndicator = withStatusIndicator(RemoteWritesContent);
const RemoteWrites: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<RemoteWriteRes>(`${pathPrefix}/${API_PATH}/remote_writes`);

  return <RemoteWritesWithStatusIndicator response={response} error={error} isLoading={isLoading} />;
};

export default RemoteWrites;
