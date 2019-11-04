import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Button } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import CopyToClipboard from 'react-copy-to-clipboard';
import { useFetch } from '../utils/useFetch';
import PathPrefixProps from '../PathPrefixProps';

import './Config.css';

const Config: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
  const { response, error } = useFetch(`${pathPrefix}/api/v1/status/config`);
  const [copied, setCopied] = useState(false);

  const config = response && response.data.yaml;
  return (
    <>
      <h2>
        Configuration&nbsp;
        <CopyToClipboard
          text={config ? config! : ''}
          onCopy={(text, result) => {
            setCopied(result);
            setTimeout(setCopied, 1500);
          }}
        >
          <Button color="light" disabled={!config}>
            {copied ? 'Copied' : 'Copy to clipboard'}
          </Button>
        </CopyToClipboard>
      </h2>

      {error ? (
        <Alert color="danger">
          <strong>Error:</strong> Error fetching configuration: {error.message}
        </Alert>
      ) : config ? (
        <pre className="config-yaml">{config}</pre>
      ) : (
        <FontAwesomeIcon icon={faSpinner} spin />
      )}
    </>
  );
};

export default Config;
