import React, { FC } from 'react';

export const AgentContent: FC = () => {
  return (
    <>
      <h2>Prometheus Agent</h2>
      <p>
        This Prometheus instance runs in <strong>agent mode</strong>. In this mode, Prometheus is only used to scrape
        discovered targets and forward them to remote write endpoints.
      </p>
      <p>The PromQL editor, graph console, recording rules and alerting page are not available in this mode.</p>
    </>
  );
};

const Agent: FC = () => {
  return <AgentContent />;
};

export default Agent;
