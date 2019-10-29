import React, { useState, useEffect } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

const useFetch = <R extends {}>(urls: string[], options?: any) => {
  const [response, setResponse] = useState<R[]>();
  const [error, setError] = useState<Error>();
  const [isLoading, setIsLoading] = useState<boolean>();

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        const responses = await Promise.all(
          urls.map(async url => {
            const response = await fetch(url)
            if (response.ok) {
              const result = await response.json()
              return result.data
            }
          })
        );
        setResponse(responses);
        setIsLoading(false);
      } catch (error) {
        setError(error);
      }
    };
    fetchData();
  }, [urls]);

  return {
    response,
    error,
    loading: isLoading ? {
      spiner: (
        <FontAwesomeIcon
          size="3x"
          icon={faSpinner}
          spin
          className="position-absolute"
          style={{ transform: 'translate(-50%, -50%)', top: '50%', left: '50%' }}
        />
      ),
    } : null,
  };
};

export default useFetch;
