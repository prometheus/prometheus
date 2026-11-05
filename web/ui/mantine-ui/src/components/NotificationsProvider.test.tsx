// Copyright The Prometheus Authors

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import {
  fetchEventSource,
  FetchEventSourceInit,
} from "@microsoft/fetch-event-source";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import NotificationsProvider from "./NotificationsProvider";
import { useNotifications } from "../state/useNotifications";

vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "/prometheus" }),
}));
vi.mock("@microsoft/fetch-event-source", () => ({
  fetchEventSource: vi.fn().mockResolvedValue(undefined),
}));
let client: QueryClient;
const notification = {
  text: "notice",
  date: "2026-01-01",
  active: true,
  modified: false,
};
const Consumer = () => <output>{JSON.stringify(useNotifications())}</output>;
function read() {
  return JSON.parse(screen.getByRole("status").textContent!);
}
function mount() {
  const view = render(
    <QueryClientProvider client={client}>
      <NotificationsProvider>
        <Consumer />
      </NotificationsProvider>
    </QueryClientProvider>,
  );
  const options = vi.mocked(fetchEventSource).mock
    .calls[0][1] as FetchEventSourceInit;
  return { ...view, options };
}
beforeEach(() => {
  client = new QueryClient();
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("notification transports", () => {
  it("deduplicates live messages, removes inactive notices, retries errors, and cleans up", async () => {
    const { options, unmount } = mount();
    await act(async () => options.onopen!(new Response(null, { status: 200 })));
    const message = (active: boolean) =>
      options.onmessage!({
        data: JSON.stringify({ ...notification, active }),
        event: "",
        id: "",
      });
    act(() => {
      message(true);
      message(true);
    });
    expect(read().notifications).toEqual([notification]);
    act(() => message(false));
    expect(read().notifications).toEqual([]);
    act(() => {
      expect(options.onerror!(new Error("disconnected"))).toBe(5000);
    });
    expect(read().isConnectionError).toBe(true);
    await act(async () => options.onopen!(new Response(null, { status: 200 })));
    expect(read().isConnectionError).toBe(false);
    unmount();
    expect(options.signal!.aborted).toBe(true);
  });

  it("switches a 204 stream to polling and derives polling data and errors", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue({
        ok: true,
        json: async () => ({ status: "success", data: [notification] }),
      });
    vi.stubGlobal("fetch", fetch);
    const { options } = mount();
    expect(fetch).not.toHaveBeenCalled();
    await act(async () => options.onopen!(new Response(null, { status: 204 })));
    expect(options.signal!.aborted).toBe(true);
    await waitFor(() => expect(read().notifications).toEqual([notification]));
    expect(fetch.mock.calls[0][0]).toBe("/prometheus/api/v1/notifications");
    fetch.mockRejectedValue(new Error("poll failed"));
    await act(async () => client.refetchQueries());
    await waitFor(() => expect(read().isConnectionError).toBe(true));
    expect(read().notifications).toEqual([notification]);
    fetch.mockResolvedValue({
      ok: true,
      json: async () => ({ status: "success", data: [] }),
    });
    await act(async () => client.refetchQueries());
    await waitFor(() =>
      expect(read()).toEqual({ notifications: [], isConnectionError: false }),
    );
  });
});
