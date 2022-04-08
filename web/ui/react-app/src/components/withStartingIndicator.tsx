import React, { FC, ComponentType } from 'react';
import { Progress, Alert } from 'reactstrap';

import { useFetchReadyInterval } from '../hooks/useFetch';
import { WALReplayData } from '../types/types';
import { usePathPrefix } from '../contexts/PathPrefixContext';

interface StartingContentProps {
  isUnexpected: boolean;
  status?: WALReplayData;
}

export const StartingContent: FC<StartingContentProps> = ({ status, isUnexpected }) => {
  if (isUnexpected) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Server is not responding
      </Alert>
    );
  }

  return (
    <div className="text-center m-3">
      <div className="m-4">
        <h2>Starting up...</h2>
        {status && status.max > 0 ? (
          <div>
            <p>
              Replaying WAL ({status.current}/{status.max})
            </p>
            <Progress
              animated
              value={status.current - status.min + 1}
              min={status.min}
              max={status.max - status.min + 1}
              color={status.max === status.current ? 'success' : undefined}
              style={{ width: '10%', margin: 'auto' }}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
};

export const withStartingIndicator =
  <T extends Record<string, unknown>>(Page: ComponentType<T>): FC<T> =>
  ({ ...rest }) => {
    const pathPrefix = usePathPrefix();
    const { ready, walReplayStatus, isUnexpected } = useFetchReadyInterval(pathPrefix);

    if (ready || isUnexpected) {
      return <Page {...(rest as T)} />;
    }

    return <StartingContent isUnexpected={isUnexpected} status={walReplayStatus.data} />;
  };
