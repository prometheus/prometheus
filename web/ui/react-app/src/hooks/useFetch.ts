import { useState, useEffect } from 'react';
import { API_PATH } from '../constants/constants';

export type APIResponse<T> = { status: string; data: T };

export interface FetchState<T> {
  response: APIResponse<T>;
  error?: Error;
  isLoading: boolean;
}

export interface FetchStateReady {
  ready: boolean;
  isResponding: boolean;
  isLoading: boolean;
}

export type WalReplayData = {
  first: number;
  last: number;
  read: number;
  started: boolean;
  done: boolean;
};

export type WalReplayStatus = {
  data?: WalReplayData;
};

export interface FetchStateReadyInterval {
  ready: boolean;
  isResponding: boolean;
  walReplayStatus: WalReplayStatus;
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
  const [isResponding, setIsResponding] = useState<boolean>(true);
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
          setIsResponding(false);
        } else {
          setIsResponding(true);
        }

        setIsLoading(false);
      } catch (error) {}
    };
    fetchData();
  }, [pathPrefix, options]);
  return { ready, isResponding, isLoading };
};

// This is used on the starting page to periodically check if the server is ready yet,
// and check the status of the WAL replay.
export const useFetchReadyInterval = (pathPrefix: string, options?: RequestInit): FetchStateReadyInterval => {
  const [ready, setReady] = useState<boolean>(false);
  const [isResponding, setIsResponding] = useState<boolean>(true);
  const [walReplayStatus, setWalReplayStatus] = useState<WalReplayStatus>({} as any);

  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        var res = await fetch(`${pathPrefix}/-/ready`, { cache: 'no-store', credentials: 'same-origin', ...options });
        if (res.status === 200) {
          setReady(true);
          clearInterval(interval);
          return;
        }
        if (res.status !== 503) {
          setIsResponding(false);
        } else {
          setIsResponding(true);

          res = await fetch(`${pathPrefix}/${API_PATH}/status/walreplay`, { cache: 'no-store', credentials: 'same-origin' });
          if (res.ok) {
            const data = (await res.json()) as WalReplayStatus;
            setWalReplayStatus(data);
          }
        }
      } catch (error) {}
    }, 1000);

    return () => clearInterval(interval);
  }, [pathPrefix, options]);
  return { ready, isResponding, walReplayStatus };
};
