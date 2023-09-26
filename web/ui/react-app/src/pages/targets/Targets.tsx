import React, { FC, useCallback, useState } from 'react';
import ScrapePoolList, { ScrapePoolNamesListProps } from './ScrapePoolList';
import { API_PATH } from '../../constants/constants';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { setQueryParam, getQueryParam } from '../../utils/index';

const ScrapePoolListWithStatusIndicator = withStatusIndicator(ScrapePoolList);

const scrapePoolQueryParam = 'scrapePool';

const Targets: FC = () => {
  // get the initial name of selected scrape pool from query args
  const scrapePool = getQueryParam(scrapePoolQueryParam) || null;

  const [selectedPool, setSelectedPool] = useState<string | null>(scrapePool);

  const onPoolSelect = useCallback(
    (name: string) => {
      setSelectedPool(name);
      setQueryParam(scrapePoolQueryParam, name);
    },
    [setSelectedPool]
  );

  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ScrapePoolNamesListProps>(`${pathPrefix}/${API_PATH}/scrape_pools`);
  const { status: responseStatus } = response;
  const badResponse = responseStatus !== 'success' && responseStatus !== 'start fetching';

  return (
    <>
      <h2>Targets</h2>
      <ScrapePoolListWithStatusIndicator
        error={badResponse ? new Error(responseStatus) : error}
        isLoading={isLoading}
        componentTitle="Targets"
        selectedPool={selectedPool}
        onPoolSelect={onPoolSelect}
        {...response.data}
      />
    </>
  );
};

export default Targets;
