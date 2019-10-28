import React, { FC, useEffect, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Alert, Button } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import CopyToClipboard from 'react-copy-to-clipboard';

import './Config.css';

const Config: FC<RouteComponentProps> = () => {
  const [config, setConfig] = useState(null);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    fetch('../api/v1/status/config')
      .then(res => res.json())
      .then(res => setConfig(res.data.yaml))
      .catch(error => setError(error.message));
  }, []);

  return (
    <>
      <h2>
        Configuration&nbsp;
        <CopyToClipboard
          text={config ? config! : ''}
          onCopy={(text, result) => { setCopied(result); setTimeout(setCopied, 1500);}}
        >
          <Button color="light" disabled={!config}>
            {copied ? 'Copied' : 'Copy to clipboard'}
          </Button>
        </CopyToClipboard>
      </h2>

      {error
        ? <Alert color="danger"><strong>Error:</strong> Error fetching configuration: {error}</Alert>
        : config
          ? <pre className="config-yaml">{config}</pre>
          : <FontAwesomeIcon icon={faSpinner} spin />
      }
    </>
  )
}

export default Config;
