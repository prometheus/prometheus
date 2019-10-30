import React, { useState, useEffect } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

const useFetch = <R extends any>(urls: string | string[], options?: any) => {
  const [response, setResponse] = useState<typeof urls extends string[] ? R[] : R>();
  const [error, setError] = useState<Error>();

  useEffect(() => {
    const fetchData = async () => {
      try {
        if (Array.isArray(urls)) {
          const responses: any = await Promise.all(
            urls
              .map(async url => {
                const response = await fetch(url);
                if (response.ok) {
                  const result = await response.json();
                  return result.data;
                }
              })
              .filter(Boolean)
          );
          setResponse(responses);
        } else {
          const response = await fetch(urls);
          if (response.ok) {
            const result = await response.json();
            setResponse(result.data);
          }
        }
      } catch (error) {
        setError(error);
      }
    };
    fetchData();
  }, [urls]);

  return {
    response,
    error,
    spinner:
      !response || !response.length ? (
        <FontAwesomeIcon
          size="3x"
          icon={faSpinner}
          spin
          className="position-absolute"
          style={{ transform: 'translate(-50%, -50%)', top: '50%', left: '50%' }}
        />
      ) : null,
  };
};

export default useFetch;
