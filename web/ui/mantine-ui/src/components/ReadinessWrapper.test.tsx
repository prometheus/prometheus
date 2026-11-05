// Copyright The Prometheus Authors

import { Suspense } from "react";
import { MantineProvider } from "@mantine/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ErrorBoundary from "./ErrorBoundary";
import ReadinessWrapper from "./ReadinessWrapper";

vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({
    pathPrefix: "/prometheus",
    ready: false,
    agentMode: false,
  }),
}));
vi.mock("../state/hooks", () => ({ useAppDispatch: () => vi.fn() }));

describe("readiness query errors", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = new QueryClient();
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.stubGlobal(
      "matchMedia",
      vi
        .fn()
        .mockReturnValue({
          matches: false,
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
        }),
    );
  });

  afterEach(() => {
    cleanup();
    client.clear();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it.each([
    { name: "network error", error: new TypeError("offline") },
    { name: "non-Error rejection", error: "offline" },
    { name: "unexpected HTTP status", status: 500 },
  ])("preserves the cause of $name", async (fixture) => {
    const fetch = vi.fn();
    if ("status" in fixture) {
      fetch.mockResolvedValue({
        status: fixture.status,
        statusText: "Internal Server Error",
      });
    } else {
      fetch.mockRejectedValue(fixture.error);
    }
    vi.stubGlobal("fetch", fetch);

    render(
      <MantineProvider>
        <MemoryRouter>
          <QueryClientProvider client={client}>
            <ErrorBoundary>
              <Suspense fallback="Loading">
                <ReadinessWrapper>Ready</ReadinessWrapper>
              </Suspense>
            </ErrorBoundary>
          </QueryClientProvider>
        </MemoryRouter>
      </MantineProvider>,
    );

    expect(
      await screen.findByText(/Unexpected error while fetching ready status/),
    ).toBeInTheDocument();
    const error = client.getQueryState(["ready", 0])?.error;
    expect(error?.message).toBe("Unexpected error while fetching ready status");
    if ("status" in fixture) {
      expect(error?.cause).toEqual(new Error("Internal Server Error"));
    } else {
      expect(error?.cause).toBe(fixture.error);
    }
  });
});
