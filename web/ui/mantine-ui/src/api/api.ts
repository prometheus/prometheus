import { QueryKey, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useSettings } from "../state/settingsSlice";
import { SearchNDJSONResponse } from "./responseTypes/search";

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

const createQueryFn =
  <T>({
    pathPrefix,
    path,
    params,
    recordResponseTime,
  }: {
    pathPrefix: string;
    path: string;
    params?: Record<string, string>;
    recordResponseTime?: (time: number) => void;
  }) =>
  async ({ signal }: { signal: AbortSignal }) => {
    const queryString = params
      ? `?${new URLSearchParams(params).toString()}`
      : "";

    try {
      const startTime = Date.now();

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

      if (recordResponseTime) {
        recordResponseTime(Date.now() - startTime);
      }

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
  key?: QueryKey;
  path: string;
  params?: Record<string, string>;
  enabled?: boolean;
  refetchInterval?: false | number;
  recordResponseTime?: (time: number) => void;
};

export const useAPIQuery = <T>({
  key,
  path,
  params,
  enabled,
  recordResponseTime,
  refetchInterval,
}: QueryOptions) => {
  const { pathPrefix } = useSettings();

  return useQuery<SuccessAPIResponse<T>>({
    queryKey: key !== undefined ? key : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: refetchInterval,
    gcTime: 0,
    enabled,
    queryFn: createQueryFn({ pathPrefix, path, params, recordResponseTime }),
  });
};

// parseNDJSON reads a streaming NDJSON response and collects all results and
// the final trailer's has_more flag.
const parseNDJSON = async <T>(
  response: Response
): Promise<SearchNDJSONResponse<T>> => {
  const text = await response.text();
  const lines = text.trim().split("\n").filter(Boolean);
  const results: T[] = [];
  let hasMore = false;
  const warnings: string[] = [];

  for (const line of lines) {
    const obj = JSON.parse(line);
    if ("status" in obj) {
      if (obj.status === "error") {
        throw new Error(obj.error ?? "unknown search error");
      }
      hasMore = obj.has_more ?? false;
      if (obj.warnings) warnings.push(...obj.warnings);
    } else if ("results" in obj) {
      results.push(...obj.results);
      if (obj.warnings) warnings.push(...obj.warnings);
    }
  }

  return { results, hasMore, warnings };
};

// useSearchQuery fetches a search endpoint that returns NDJSON and exposes
// the results together with the has_more flag from the trailer.
export const useSearchQuery = <T>({
  path,
  params,
  enabled,
}: {
  path: string;
  params: Record<string, string>;
  enabled?: boolean;
}) => {
  const { pathPrefix } = useSettings();

  return useQuery<SearchNDJSONResponse<T>>({
    queryKey: ["search", path, params],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    staleTime: 10000,
    enabled,
    queryFn: async ({ signal }) => {
      const qs = `?${new URLSearchParams(params).toString()}`;
      const res = await fetch(`${pathPrefix}/${API_PATH}${path}${qs}`, {
        cache: "no-store",
        credentials: "same-origin",
        signal,
      });
      if (!res.ok) {
        throw new Error(res.statusText);
      }
      return parseNDJSON<T>(res);
    },
  });
};

export const useSuspenseAPIQuery = <T>({ key, path, params }: QueryOptions) => {
  const { pathPrefix } = useSettings();

  return useSuspenseQuery<SuccessAPIResponse<T>>({
    queryKey: key !== undefined ? key : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    staleTime: Infinity, // Required for suspense queries since the component is briefly unmounted when loading the data, which together with a gcTime of 0 will cause the data to be garbage collected before it can be used.
    gcTime: 0,
    queryFn: createQueryFn({ pathPrefix, path, params }),
  });
};
