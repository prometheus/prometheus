import { useState, useEffect } from 'react';

export type APIResponse<T> = { status: string; data: T };

export interface FetchState<T> {
  response: APIResponse<T>;
  error?: Error;
  isLoading: boolean;
}

export const useFetch = <T extends {}>(url: string, options?: RequestInit, includeTime?: boolean): FetchState<T> => {
  const [response, setResponse] = useState<APIResponse<T>>({ status: 'start fetching' } as any);
  const [error, setError] = useState<Error>();
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        // By adding the current timestamp to the url we are preventing the response from being cached
        if (includeTime) {
          // eslint-disable-next-line
          url += `?_=${new Date().getTime()}`
        }

        const res = await fetch(url, { cache: 'no-store', credentials: 'same-origin', ...options });
        if (!res.ok) {
          throw new Error(res.statusText);
        }
        const json = (await res.json()) as APIResponse<T>;
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
