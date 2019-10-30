import { useState, useEffect } from 'react';

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

  return { response, error, isLoading: !response || !response.length };
};

export default useFetch;
