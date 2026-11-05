import {
  keepPreviousData,
  QueryKey,
  useQuery,
  UseQueryResult,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useSettings } from "../state/settingsSlice";

export const API_PATH = "api/v1";

export type SuccessAPIResponse<T> = {
  status: "success";
  data: T;
  warnings?: string[];
  infos?: string[];
};

export type ErrorAPIResponse = {
  status: "error";
  errorType: string;
  error: string;
};

export type APIResponse<T> = SuccessAPIResponse<T> | ErrorAPIResponse;

/** APIQueryMetadata describes the request that produced a successful response. */
export type APIQueryMetadata = {
  params: Readonly<Record<string, string>>;
  responseTimeMs: number;
  receivedAtMs: number;
};

type QueryResult<T> = {
  response: SuccessAPIResponse<T>;
  metadata: APIQueryMetadata;
};

type QueryParams =
  Record<string, string> | ((requestTimeMs: number) => Record<string, string>);

const createQueryFn =
  <T>({
    pathPrefix,
    path,
    params,
    recordResponseTime,
  }: {
    pathPrefix: string;
    path: string;
    params?: QueryParams;
    recordResponseTime?: (time: number) => void;
  }) =>
  async ({ signal }: { signal: AbortSignal }) => {
    try {
      const startTime = Date.now();
      const resolvedParams =
        typeof params === "function" ? params(startTime) : params;
      const requestParams = { ...resolvedParams };
      const queryString = resolvedParams
        ? `?${new URLSearchParams(requestParams).toString()}`
        : "";

      const res = await fetch(
        `${pathPrefix}/${API_PATH}${path}${queryString}`,
        {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        },
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
      const receivedAtMs = Date.now();
      const responseTimeMs = receivedAtMs - startTime;

      if (recordResponseTime) {
        recordResponseTime(responseTimeMs);
      }

      if (apiRes.status === "error") {
        throw new Error(
          apiRes.error !== undefined
            ? apiRes.error
            : 'missing "error" field in response JSON',
        );
      }

      return {
        response: apiRes,
        metadata: { params: requestParams, responseTimeMs, receivedAtMs },
      };
    } catch (error) {
      if (!(error instanceof Error)) {
        throw new Error("Unknown error", { cause: error });
      }

      switch (error.name) {
        case "TypeError":
          throw new Error("Network error or unable to reach the server", {
            cause: error,
          });
        case "SyntaxError":
          throw new Error("Invalid JSON response", { cause: error });
        default:
          throw error;
      }
    }
  };

type QueryOptions = {
  path: string;
  enabled?: boolean;
  refetchInterval?: false | number;
  recordResponseTime?: (time: number) => void;
  keepPreviousData?: boolean;
} & (
  | { key?: QueryKey; params?: Record<string, string> }
  | { key: QueryKey; params: (requestTimeMs: number) => Record<string, string> }
);

/**
 * Queries the API, optionally selecting a response with its request metadata.
 * Deferred params receive the fetch start time in milliseconds and require an explicit key.
 */
export function useAPIQuery<T>(
  options: QueryOptions,
): UseQueryResult<SuccessAPIResponse<T>>;
export function useAPIQuery<T, D>(
  options: QueryOptions & {
    select: (response: SuccessAPIResponse<T>, metadata: APIQueryMetadata) => D;
  },
): UseQueryResult<D>;
export function useAPIQuery<T, D>({
  key,
  path,
  params,
  enabled,
  recordResponseTime,
  refetchInterval,
  keepPreviousData: retainPreviousData,
  select,
}: QueryOptions & {
  select?: (response: SuccessAPIResponse<T>, metadata: APIQueryMetadata) => D;
}): UseQueryResult<SuccessAPIResponse<T> | D> {
  const { pathPrefix } = useSettings();

  return useQuery<QueryResult<T>, Error, SuccessAPIResponse<T> | D>({
    queryKey: key !== undefined ? key : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: refetchInterval,
    gcTime: 0,
    enabled,
    queryFn: createQueryFn<T>({ pathPrefix, path, params, recordResponseTime }),
    placeholderData: retainPreviousData ? keepPreviousData : undefined,
    select: (result) =>
      select ? select(result.response, result.metadata) : result.response,
  });
}

export const useSuspenseAPIQuery = <T>({ key, path, params }: QueryOptions) => {
  const { pathPrefix } = useSettings();

  return useSuspenseQuery<QueryResult<T>, Error, SuccessAPIResponse<T>>({
    queryKey: key !== undefined ? key : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    staleTime: Infinity, // Required for suspense queries since the component is briefly unmounted when loading the data, which together with a gcTime of 0 will cause the data to be garbage collected before it can be used.
    gcTime: 0,
    queryFn: createQueryFn<T>({ pathPrefix, path, params }),
    select: (result) => result.response,
  });
};
