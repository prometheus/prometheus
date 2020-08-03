import React, { FC } from 'react';
import { Badge, Alert } from 'reactstrap';

export interface EndpointLinkProps {
  endpoint: string;
  globalUrl: string;
}

const EndpointLink: FC<EndpointLinkProps> = ({ endpoint, globalUrl }) => {
  let url: URL;
  try {
    url = new URL(endpoint);
  } catch (e) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> {e.message}
      </Alert>
    );
  }

  const { host, pathname, protocol, searchParams }: URL = url;
  const params = Array.from(searchParams.entries());

  return (
    <>
      <a href={globalUrl}>{`${protocol}//${host}${pathname}`}</a>
      {params.length > 0 ? <br /> : null}
      {params.map(([labelName, labelValue]: [string, string]) => {
        return (
          <Badge color="primary" className={`mr-1 ${labelName}`} key={labelName}>
            {`${labelName}="${labelValue}"`}
          </Badge>
        );
      })}
    </>
  );
};

export default EndpointLink;
