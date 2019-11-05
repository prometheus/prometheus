import React, { FC, Fragment } from 'react';
import { Alert } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

interface StatusIndicatorProps {
  error?: string;
  hasData?: boolean;
}

export const StatusIndicator: FC<StatusIndicatorProps> = ({ error, hasData, children }) => {
  if (error) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> {error}
      </Alert>
    );
  }

  if (!hasData) {
    return (
      <FontAwesomeIcon
        size="3x"
        icon={faSpinner}
        spin
        className="position-absolute"
        style={{ transform: 'translate(-50%, -50%)', top: '50%', left: '50%' }}
      />
    );
  }
  return <Fragment>{children}</Fragment>;
};
