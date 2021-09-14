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
  } catch (err: unknown) {
    const error = err as Error;
    return (
      <Alert color="danger">
        <strong>Error:</strong> {error.message}
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
          <Badge color="primary" className="mr-1" key={`${labelName}/${labelValue}`}>
            {`${labelName}="${labelValue}"`}
          </Badge>
        );
      })}
    </>
  );
};

export default EndpointLink;
