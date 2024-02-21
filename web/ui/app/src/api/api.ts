import { useSuspenseQuery } from "@tanstack/react-query";

export const API_PATH = "api/v1";

export type APIResponse<T> = { status: string; data: T };

export const useSuspenseAPIQuery = <T>(path: string) =>
  useSuspenseQuery<{ data: T }>({
    queryKey: [path],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: () =>
      fetch(`/${API_PATH}/${path}`, {
        cache: "no-store",
        credentials: "same-origin",
      })
        // Introduce 3 seconds delay to simulate slow network.
        // .then(
        //   (res) =>
        //     new Promise<typeof res>((resolve) =>
        //       setTimeout(() => resolve(res), 2000)
        //     )
        // )
        .then((res) => {
          if (!res.ok) {
            throw new Error(res.statusText);
          }
          return res;
        })
        .then((res) => res.json() as Promise<APIResponse<T>>),
  });
