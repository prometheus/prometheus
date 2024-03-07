import { useQuery, useSuspenseQuery } from "@tanstack/react-query";

export const API_PATH = "api/v1";

export type SuccessAPIResponse<T> = {
  status: "success";
  data: T;
  warnings?: string[];
};

export type ErrorAPIResponse = {
  status: "error";
  errorType: string;
  error: string;
};

export type APIResponse<T> = SuccessAPIResponse<T> | ErrorAPIResponse;

export const useAPIQuery = <T>({
  key,
  path,
  params,
  enabled,
}: {
  key?: string;
  path: string;
  params?: Record<string, string>;
  enabled?: boolean;
}) =>
  useQuery<APIResponse<T>>({
    queryKey: [key || path],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled,
    queryFn: async ({ signal }) => {
      const queryString = params
        ? `?${new URLSearchParams(params).toString()}`
        : "";
      return (
        fetch(`/${API_PATH}/${path}${queryString}`, {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        })
          // TODO: think about how to check API errors here, if this code remains in use.
          .then((res) => {
            if (!res.ok) {
              throw new Error(res.statusText);
            }
            return res;
          })
          .then((res) => res.json() as Promise<APIResponse<T>>)
      );
    },
  });

export const useSuspenseAPIQuery = <T>(
  path: string,
  params?: Record<string, string>
) =>
  useSuspenseQuery<SuccessAPIResponse<T>>({
    queryKey: [path],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: ({ signal }) => {
      const queryString = params
        ? `?${new URLSearchParams(params).toString()}`
        : "";
      return (
        fetch(`/${API_PATH}/${path}${queryString}`, {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        })
          // Introduce 3 seconds delay to simulate slow network.
          // .then(
          //   (res) =>
          //     new Promise<typeof res>((resolve) =>
          //       setTimeout(() => resolve(res), 2000)
          //     )
          // )
          // TODO: think about how to check API errors here, if this code remains in use.
          .then((res) => {
            if (!res.ok) {
              throw new Error(res.statusText);
            }
            return res;
          })
          .then((res) => res.json() as Promise<SuccessAPIResponse<T>>)
      );
    },
  });
