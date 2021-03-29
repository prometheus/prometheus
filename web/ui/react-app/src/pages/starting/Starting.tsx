import React, { FC, useEffect } from 'react';
import { RouteComponentProps, navigate } from '@reach/router';
import { Progress, Alert } from 'reactstrap';
import Logo from './Logo';

import { useFetchReadyInterval, WalReplayData } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';

interface StartingContentProps {
  isResponding: boolean;
  status?: WalReplayData;
}

export const StartingContent: FC<StartingContentProps> = ({ status, isResponding }) => {
  if (!isResponding) {
    return (
      <Alert color="danger">
        <strong>Error:</strong> Server is not responding
      </Alert>
    );
  }

  return (
    <div className="text-center m-3">
      <Logo height="400" />
      <div className="m-4">
        <h2>Starting up...</h2>
        {status?.started && !status?.done ? (
          <div className="mx-5">
            <p>
              Replaying Wal ({status?.read - status?.first}/{status?.last - status?.first})
            </p>
            <Progress
              animated
              max={status?.last - status?.first}
              value={status?.read - status?.first}
              color={status?.done ? 'success' : undefined}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
};

const Starting: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { ready, walReplayStatus, isResponding } = useFetchReadyInterval(pathPrefix);

  useEffect(() => {
    if (ready) {
      navigate('/');
    }
  }, [ready]);

  return <StartingContent isResponding={isResponding} status={walReplayStatus.data} />;
};

export default Starting;
