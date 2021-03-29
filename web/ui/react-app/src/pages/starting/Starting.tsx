import React, { FC, useEffect } from 'react';
import { RouteComponentProps, navigate } from '@reach/router';
import { Progress, Alert } from 'reactstrap';

import { useFetchReadyInterval, WALReplayData } from '../../hooks/useFetch';
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
        {status?.started && !status?.done ? (
          <div>
            <p>
              Replaying WAL ({status?.read - status?.first}/{status?.last - status?.first})
            </p>
            <Progress
              animated
              max={status?.last - status?.first}
              value={status?.read - status?.first}
              color={status?.done ? 'success' : undefined}
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
