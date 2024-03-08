import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useAppSelector } from "../state/hooks";

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

const createQueryFn =
  <T>({
    pathPrefix,
    path,
    params,
  }: {
    pathPrefix: string;
    path: string;
    params?: Record<string, string>;
  }) =>
  async ({ signal }: { signal: AbortSignal }) => {
    const queryString = params
      ? `?${new URLSearchParams(params).toString()}`
      : "";

    try {
      const res = await fetch(
        `${pathPrefix}/${API_PATH}${path}${queryString}`,
        {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        }
      );

      if (
        !res.ok &&
        !res.headers.get("content-type")?.startsWith("application/json")
      ) {
        // For example, Prometheus may send a 503 Service Unavailable response
        // with a "text/plain" content type when it's starting up. But the API
        // may also respond with a JSON error message and the same error code.
        throw new Error(res.statusText);
      }

      const apiRes = (await res.json()) as APIResponse<T>;

      if (apiRes.status === "error") {
        throw new Error(
          apiRes.error !== undefined
            ? apiRes.error
            : 'missing "error" field in response JSON'
        );
      }

      return apiRes as SuccessAPIResponse<T>;
    } catch (error) {
      if (!(error instanceof Error)) {
        throw new Error("Unknown error");
      }

      switch (error.name) {
        case "TypeError":
          throw new Error("Network error or unable to reach the server");
        case "SyntaxError":
          throw new Error("Invalid JSON response");
        default:
          throw error;
      }
    }
  };

type QueryOptions = {
  key?: string;
  path: string;
  params?: Record<string, string>;
  enabled?: boolean;
};

export const useAPIQuery = <T>({
  key,
  path,
  params,
  enabled,
}: QueryOptions) => {
  const pathPrefix = useAppSelector((state) => state.settings.pathPrefix);

  return useQuery<SuccessAPIResponse<T>>({
    queryKey: key ? [key] : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled,
    queryFn: createQueryFn({ pathPrefix, path, params }),
  });
};

export const useSuspenseAPIQuery = <T>({ key, path, params }: QueryOptions) => {
  const pathPrefix = useAppSelector((state) => state.settings.pathPrefix);

  return useSuspenseQuery<SuccessAPIResponse<T>>({
    queryKey: key ? [key] : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: createQueryFn({ pathPrefix, path, params }),
  });
};
