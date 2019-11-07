import { useState, useEffect } from 'react';

const useFetches = <R extends any>(urls: string[], options?: RequestInit) => {
  if (!urls.length) {
    throw new Error("Doesn't have url to fetch.");
  }
  const [response, setResponse] = useState<R[]>();
  const [error, setError] = useState<Error>();

  useEffect(() => {
    const fetchData = async () => {
      try {
        const responses: R[] = await Promise.all(
          urls
            .map(async url => {
              const res = await fetch(url, options);
              if (!res.ok) {
                throw new Error(res.statusText);
              }
              const result = await res.json();
              return result.data;
            })
            .filter(Boolean) // Remove falsy values
        );
        setResponse(responses);
      } catch (error) {
        setError(error);
      }
    };
    fetchData();
  }, [urls, options]);

  return { response, error, isLoading: !response || !response.length };
};

export default useFetches;
