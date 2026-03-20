import React, { FC } from 'react';

const Agent: FC = () => {
  return (
    <>
      <h2>Prometheus Agent</h2>
      <p>
        This Prometheus instance is running in <strong>agent mode</strong>. In this mode, Prometheus is only used to scrape
        discovered targets and forward the scraped metrics to remote write endpoints.
      </p>
      <p>Some features are not available in this mode, such as querying and alerting.</p>
    </>
  );
};

export default Agent;
