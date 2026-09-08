// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAPIQuery } from "./api";

vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "/prometheus" }),
}));

describe("API query errors", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = new QueryClient();
  });

  afterEach(() => {
    cleanup();
    client.clear();
    vi.unstubAllGlobals();
  });

  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
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
});
