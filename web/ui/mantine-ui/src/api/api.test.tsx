// Copyright The Prometheus Authors

import { PropsWithChildren, Suspense } from "react";
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  expectTypeOf,
  it,
  vi,
} from "vitest";
import {
  APIQueryMetadata,
  SuccessAPIResponse,
  useAPIQuery,
  useSuspenseAPIQuery,
} from "./api";

vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "/prometheus" }),
}));

describe("API queries", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = new QueryClient();
  });

  afterEach(() => {
    cleanup();
    client.clear();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    onlineManager.setOnline(true);
  });

  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>
      <Suspense>{children}</Suspense>
    </QueryClientProvider>
  );

  it.each([
    {
      name: "network failure",
      source: "fetch",
      error: new TypeError("offline"),
      message: "Network error or unable to reach the server",
      wrapped: true,
    },
    {
      name: "malformed JSON",
      source: "json",
      error: new SyntaxError("invalid JSON"),
      message: "Invalid JSON response",
      wrapped: true,
    },
    {
      name: "non-Error rejection",
      source: "fetch",
      error: "offline",
      message: "Unknown error",
      wrapped: true,
    },
    {
      name: "null rejection",
      source: "fetch",
      error: null,
      message: "Unknown error",
      wrapped: true,
    },
    {
      name: "ordinary error",
      source: "fetch",
      error: new Error("request failed"),
      message: "request failed",
      wrapped: false,
    },
  ])(
    "preserves the cause of $name",
    async ({ source, error, message, wrapped }) => {
      const fetch = vi.fn();
      if (source === "json") {
        fetch.mockResolvedValue({
          ok: true,
          json: vi.fn().mockRejectedValue(error),
        });
      } else {
        fetch.mockRejectedValue(error);
      }
      vi.stubGlobal("fetch", fetch);

      const { result } = renderHook(() => useAPIQuery({ path: "/test" }), {
        wrapper,
      });
      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error?.message).toBe(message);
      if (wrapped) {
        expect(result.current.error?.cause).toBe(error);
      } else {
        expect(result.current.error).toBe(error);
      }
    },
  );

  it("returns successful responses unchanged", async () => {
    const response = { status: "success", data: { value: 1 } };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, json: async () => response }),
    );
    const { result } = renderHook(() => useAPIQuery({ path: "/test" }), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBe(response);
  });

  it.each<Record<string, string> | undefined>([
    undefined,
    {},
    { query: "up + 1" },
  ])("preserves URL and fetch options for %j", async (params) => {
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: "success", data: 1 }),
    });
    vi.stubGlobal("fetch", fetch);
    const { result } = renderHook(
      () => useAPIQuery<number>({ path: "/query", params }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      `/prometheus/api/v1/query${params ? `?${new URLSearchParams(params)}` : ""}`,
      {
        cache: "no-store",
        credentials: "same-origin",
        signal: expect.any(AbortSignal),
      },
    );
    expectTypeOf(result.current.data).toEqualTypeOf<
      SuccessAPIResponse<number> | undefined
    >();
    const refetched = await result.current.refetch();
    expectTypeOf(refetched.data).toEqualTypeOf<
      SuccessAPIResponse<number> | undefined
    >();
  });

  it("evaluates deferred params only when an enabled, online request starts", async () => {
    onlineManager.setOnline(false);
    const params = vi.fn((requestTimeMs: number) => ({
      time: `${requestTimeMs}`,
    }));
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: "success", data: 1 }),
    });
    vi.stubGlobal("fetch", fetch);
    const { result, rerender } = renderHook(
      ({ enabled }) =>
        useAPIQuery<number>({
          key: ["deferred"],
          path: "/query",
          params,
          enabled,
        }),
      { wrapper, initialProps: { enabled: false } },
    );
    expect(params).not.toHaveBeenCalled();
    rerender({ enabled: true });
    expect(result.current.fetchStatus).toBe("paused");
    expect(params).not.toHaveBeenCalled();
    vi.spyOn(Date, "now").mockReturnValue(123);
    act(() => onlineManager.setOnline(true));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(params).toHaveBeenCalledExactlyOnceWith(123);
    expect(fetch.mock.calls[0][0]).toBe("/prometheus/api/v1/query?time=123");
  });

  it("keeps fresh request metadata with identical refetch responses", async () => {
    let now = 1000;
    vi.spyOn(Date, "now").mockImplementation(() => now);
    const params = { time: "1" };
    const response = { status: "success" as const, data: 42 };
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        now += 20;
        return { ok: true, json: async () => ({ ...response }) };
      }),
    );
    const recordResponseTime = vi.fn();
    const { result } = renderHook(
      () =>
        useAPIQuery<
          number,
          { response: SuccessAPIResponse<number>; metadata: APIQueryMetadata }
        >({
          key: ["metadata"],
          path: "/query",
          params: () => params,
          recordResponseTime,
          select: (response, metadata) => ({ response, metadata }),
        }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const first = result.current.data!;
    expect(first.metadata).toEqual({
      params: { time: "1" },
      responseTimeMs: 20,
      receivedAtMs: 1020,
    });
    params.time = "2";
    now = 2000;
    let next;
    await act(async () => {
      next = await result.current.refetch();
    });
    expect(next).toBeDefined();
    expect(result.current.data!.response).toBe(first.response);
    await waitFor(() =>
      expect(result.current.data!.metadata).toEqual({
        params: { time: "2" },
        responseTimeMs: 20,
        receivedAtMs: 2020,
      }),
    );
    expect(first.metadata.params.time).toBe("1");
    expect(recordResponseTime).toHaveBeenLastCalledWith(20);
  });

  it("shares cached responses with suspense consumers without exposing the internal wrapper", async () => {
    const response = { status: "success" as const, data: 42 };
    const fetch = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => response });
    vi.stubGlobal("fetch", fetch);
    const ordinary = renderHook(
      () => useAPIQuery<number>({ path: "/shared" }),
      { wrapper },
    );
    await waitFor(() => expect(ordinary.result.current.isSuccess).toBe(true));
    const suspense = renderHook(
      () => useSuspenseAPIQuery<number>({ path: "/shared" }),
      { wrapper },
    );
    expect(suspense.result.current.data).toBe(response);
    expectTypeOf(suspense.result.current.data).toEqualTypeOf<
      SuccessAPIResponse<number>
    >();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("retains a complete previous result and ignores late replies from cancelled requests", async () => {
    const pending: Array<{
      signal: AbortSignal;
      resolve: (response: unknown) => void;
    }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url, { signal }) =>
          new Promise((resolve) => pending.push({ signal, resolve })),
      ),
    );
    const { result, rerender } = renderHook(
      ({ query }) =>
        useAPIQuery<number, { value: number; query: string }>({
          key: [query],
          path: "/query",
          params: () => ({ query }),
          keepPreviousData: true,
          select: (response, metadata) => ({
            value: response.data,
            query: metadata.params.query,
          }),
        }),
      { wrapper, initialProps: { query: "first" } },
    );
    await act(async () =>
      pending[0].resolve({
        ok: true,
        json: async () => ({ status: "success", data: 1 }),
      }),
    );
    await waitFor(() =>
      expect(result.current.data).toEqual({ value: 1, query: "first" }),
    );
    rerender({ query: "second" });
    expect(result.current.data).toEqual({ value: 1, query: "first" });
    rerender({ query: "third" });
    expect(pending[1].signal.aborted).toBe(true);
    await act(async () =>
      pending[2].resolve({
        ok: true,
        json: async () => ({ status: "success", data: 3 }),
      }),
    );
    await waitFor(() =>
      expect(result.current.data).toEqual({ value: 3, query: "third" }),
    );
    await act(async () =>
      pending[1].resolve({
        ok: true,
        json: async () => ({ status: "success", data: 2 }),
      }),
    );
    expect(result.current.data).toEqual({ value: 3, query: "third" });
  });

  it("preserves cached data alongside a failed refetch error", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ status: "success", data: 1 }),
      })
      .mockRejectedValue(new Error("failed refetch"));
    vi.stubGlobal("fetch", fetch);
    const { result } = renderHook(
      () => useAPIQuery<number>({ path: "/test" }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await act(async () => {
      const refetched = await result.current.refetch();
      expect(refetched.error?.message).toBe("failed refetch");
      expect(refetched.data?.data).toBe(1);
    });
  });
});
