import React, { FC, useEffect } from 'react';
import { RouteComponentProps, navigate } from '@reach/router';
import { Progress, Alert } from 'reactstrap';

import { useFetchReadyInterval } from '../../hooks/useFetch';
import { WALReplayData } from '../../types/types';
import { usePathPrefix } from '../../contexts/PathPrefixContext';

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
        {status?.current! > status?.min! ? (
          <div>
            <p>
              Replaying WAL ({status?.current}/{status?.max})
            </p>
            <Progress
              animated
              value={status?.current}
              min={status?.min}
              max={status?.max}
              color={status?.max === status?.current ? 'success' : undefined}
              style={{ width: '10%', margin: 'auto' }}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
};

const Starting: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { ready, walReplayStatus, isUnexpected } = useFetchReadyInterval(pathPrefix);

  useEffect(() => {
    if (ready) {
      navigate('/');
    }
  }, [ready]);

  return <StartingContent isUnexpected={isUnexpected} status={walReplayStatus.data} />;
};

export default Starting;
