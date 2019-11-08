import React, { useState, FC } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button } from 'reactstrap';
import CopyToClipboard from 'react-copy-to-clipboard';
import PathPrefixProps from '../PathPrefixProps';

import './Config.css';
import { withStatusIndicator } from '../withStatusIndicator';
import { useFetch } from '../utils/useFetch';

type YamlConfig = { yaml: string };

interface ConfigContentProps {
  error?: Error;
  data?: YamlConfig;
}

const YamlContent = ({ yaml }: YamlConfig) => <pre className="config-yaml">{yaml}</pre>;
YamlContent.displayName = 'Config';

const ConfigWithResponseIndicator = withStatusIndicator(YamlContent);

export const ConfigContent: FC<ConfigContentProps> = ({ error, data }) => {
  const [copied, setCopied] = useState(false);
  const config = data && data.yaml ? data.yaml : '';
  const hasConfig = Boolean(config && config.length);
  return (
    <>
      <h2>
        Configuration&nbsp;
        <CopyToClipboard
          text={config}
          onCopy={(_, result) => {
            setCopied(result);
            setTimeout(setCopied, 1500);
          }}
        >
          <Button color="light" disabled={!hasConfig}>
            {copied ? 'Copied' : 'Copy to clipboard'}
          </Button>
        </CopyToClipboard>
      </h2>
      <ConfigWithResponseIndicator error={error} isLoading={!hasConfig} yaml={config} />
    </>
  );
};

const Config: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch<YamlConfig>(`${pathPrefix}/api/v1/status/config`);
  return <ConfigContent error={error} data={response.data} />;
};

export default Config;
