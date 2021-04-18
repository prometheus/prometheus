import { useState, useEffect } from 'react';
import { API_PATH } from '../constants/constants';
import { WALReplayStatus } from '../types/types';

export type APIResponse<T> = { status: string; data: T };

export interface FetchState<T> {
  response: APIResponse<T>;
  error?: Error;
  isLoading: boolean;
}

export interface FetchStateReady {
  ready: boolean;
  isUnexpected: boolean;
  isLoading: boolean;
}

export interface FetchStateReadyInterval {
  ready: boolean;
  isUnexpected: boolean;
  walReplayStatus: WALReplayStatus;
}

export const useFetch = <T extends {}>(url: string, options?: RequestInit): FetchState<T> => {
  const [response, setResponse] = useState<APIResponse<T>>({ status: 'start fetching' } as any);
  const [error, setError] = useState<Error>();
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
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

export const useFetchReady = (pathPrefix: string, options?: RequestInit): FetchStateReady => {
  const [ready, setReady] = useState<boolean>(false);
  const [isUnexpected, setIsUnexpected] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        const res = await fetch(`${pathPrefix}/-/ready`, { cache: 'no-store', credentials: 'same-origin', ...options });
        if (res.status === 200) {
          setReady(true);
        }
        // The server sends back a 503 if it isn't ready,
        // if we get back anything else that means something has gone wrong.
        if (res.status !== 503) {
          setIsUnexpected(true);
        } else {
          setIsUnexpected(false);
        }

        setIsLoading(false);
      } catch (error) {
        setIsUnexpected(true);
      }
    };
    fetchData();
  }, [pathPrefix, options]);
  return { ready, isUnexpected, isLoading };
};

// This is used on the starting page to periodically check if the server is ready yet,
// and check the status of the WAL replay.
export const useFetchReadyInterval = (pathPrefix: string, options?: RequestInit): FetchStateReadyInterval => {
  const [ready, setReady] = useState<boolean>(false);
  const [isUnexpected, setIsUnexpected] = useState<boolean>(false);
  const [walReplayStatus, setWALReplayStatus] = useState<WALReplayStatus>({} as any);

  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        let res = await fetch(`${pathPrefix}/-/ready`, { cache: 'no-store', credentials: 'same-origin', ...options });
        if (res.status === 200) {
          setReady(true);
          clearInterval(interval);
          return;
        }
        if (res.status !== 503) {
          setIsUnexpected(true);
          setWALReplayStatus({ data: { last: 0, first: 0 } } as any);
        } else {
          setIsUnexpected(false);

          res = await fetch(`${pathPrefix}/${API_PATH}/status/walreplay`, { cache: 'no-store', credentials: 'same-origin' });
          if (res.ok) {
            const data = (await res.json()) as WALReplayStatus;
            setWALReplayStatus(data);
          }
        }
      } catch (error) {
        setIsUnexpected(true);
        setWALReplayStatus({ data: { last: 0, first: 0 } } as any);
      }
    }, 1000);

    return () => clearInterval(interval);
  }, [pathPrefix, options]);
  return { ready, isUnexpected, walReplayStatus };
};
