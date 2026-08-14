import { useState, useEffect } from 'react';
import { API_PATH } from '../constants/constants';
import { WALReplayStatus } from '../types/types';

export type APIResponse<T> = { status: string; data: T };

export interface FetchState<T> {
  response: APIResponse<T>;
  error?: Error;
  isLoading: boolean;
}

export interface FetchStateReadyInterval {
  ready: boolean;
  isUnexpected: boolean;
  walReplayStatus: WALReplayStatus;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const useFetch = <T extends Record<string, any>>(url: string, options?: RequestInit): FetchState<T> => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
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
      } catch (err: unknown) {
        const error = err as Error;
        setError(error);
      }
    };
    fetchData();
  }, [url, options]);
  return { response, error, isLoading };
};

let wasReady = false;

// This is used on the starting page to periodically check if the server is ready yet,
// and check the status of the WAL replay.
export const useFetchReadyInterval = (pathPrefix: string, options?: RequestInit): FetchStateReadyInterval => {
  const [ready, setReady] = useState<boolean>(false);
  const [isUnexpected, setIsUnexpected] = useState<boolean>(false);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const [walReplayStatus, setWALReplayStatus] = useState<WALReplayStatus>({} as any);

  useEffect(() => {
    if (wasReady) {
      setReady(true);
    } else {
      // This helps avoid a memory leak.
      let mounted = true;

      const fetchStatus = async () => {
        try {
          let res = await fetch(`${pathPrefix}/-/ready`, { cache: 'no-store', credentials: 'same-origin', ...options });
          if (res.status === 200) {
            if (mounted) {
              setReady(true);
            }
            wasReady = true;
            clearInterval(interval);
          } else if (res.status !== 503) {
            if (mounted) {
              setIsUnexpected(true);
            }
            clearInterval(interval);
            return;
          } else {
            if (mounted) {
              setIsUnexpected(false);
            }

            res = await fetch(`${pathPrefix}/${API_PATH}/status/walreplay`, {
              cache: 'no-store',
              credentials: 'same-origin',
            });
            if (res.ok) {
              const data = (await res.json()) as WALReplayStatus;
              if (mounted) {
                setWALReplayStatus(data);
              }
            }
          }
        } catch (error) {
          if (mounted) {
            setIsUnexpected(true);
          }
          clearInterval(interval);
          return;
        }
      };

      fetchStatus();
      const interval = setInterval(fetchStatus, 1000);
      return () => {
        clearInterval(interval);
        mounted = false;
      };
    }
  }, [pathPrefix, options]);

  return { ready, isUnexpected, walReplayStatus };
};
