import React, { FC } from 'react';
import { Alert } from 'reactstrap';

interface ErrorProps {
  summary: string;
  message?: string;
}

const Error: FC<ErrorProps> = ({ summary, message }) => {
  return (
    <Alert color="danger">
      <strong>Error:</strong> {summary}: {message}
    </Alert>
  );
};

export default Error;
