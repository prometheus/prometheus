import * as React from 'react';
import SeriesLabel from './SeriesLabel';
import ErrorAlert from '../../ErrorAlert';

export interface EndpointLinkProps {
  endpoint: string;
}

const EndpointLink: React.FC<EndpointLinkProps> = ({ endpoint }) => {
  let url: URL;
  try {
    url = new URL(endpoint);
  } catch (e) {
    return <ErrorAlert summary={e.message} />;
  }

  const { host, pathname, protocol, searchParams }: URL = url;
  const params = Array.from(searchParams.entries());

  return (
    <>
      <a href={endpoint}>{`${protocol}//${host}${pathname}`}</a>
      {params.length > 0 ? <br /> : null}
      {params.map(([labelKey, labelValue]: [string, string]) => {
        return <SeriesLabel key={labelKey} labelKey={labelKey} labelValue={labelValue} />;
      })}
    </>
  );
};

export default EndpointLink;
