import { useState, useEffect } from 'react';

export const useFetch = (url: string, options?: RequestInit) => {
  const [response, setResponse] = useState();
  const [error, setError] = useState();
  const [isLoading, setIsLoading] = useState();

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        const res = await fetch(url, options);
        if (!res.ok) {
          throw new Error(res.statusText);
        }
        const json = await res.json();
        setResponse(json);
        setIsLoading(false);
      } catch (error) {
        setError(error);
      }
    };
    fetchData();
  }, [url, options]);

  return { response, error, isLoading };
};
