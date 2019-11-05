import React, { useState, FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button } from 'reactstrap';
import CopyToClipboard from 'react-copy-to-clipboard';
import PathPrefixProps from '../PathPrefixProps';

import './Config.css';
import { Fetch, FetchState } from '../api/Fetch';
import { StatusIndicator } from '../StatusIndicator';

type YamlConfig = { yaml: string };

export const ConfigContent: FC<FetchState<YamlConfig>> = ({ error, data }) => {
  const [copied, setCopied] = useState(false);
  const config = data && data.yaml ? data.yaml : '';
  const hasConfig = Boolean(config && config.length);
  return (
    <>
      <h2>
        Configuration&nbsp;
        <CopyToClipboard
          text={config}
          onCopy={(text, result) => {
            setCopied(result);
            setTimeout(setCopied, 1500);
          }}
        >
          <Button color="light" disabled={!hasConfig}>
            {copied ? 'Copied' : 'Copy to clipboard'}
          </Button>
        </CopyToClipboard>
      </h2>
      <StatusIndicator error={error && `Error fetching configuration: ${error.message}`} hasData={hasConfig}>
        <pre className="config-yaml">{config}</pre>
      </StatusIndicator>
    </>
  );
};

const Config: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  return (
    <Fetch url={`${pathPrefix}/api/v1/status/config`}>
      {(state: FetchState<YamlConfig>) => <ConfigContent {...state} />}
    </Fetch>
  );
};

export default Config;
