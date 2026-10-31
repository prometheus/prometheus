// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProtoMetricFamily } from "../api/responseTypes/selfMetrics";
import RemoteWriteStatusPage from "./RemoteWriteStatusPage";

vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "" }),
}));

const makeQueueFamilies = ({
  remoteName = "rw-primary",
  url = "http://remote.example/api/v1/write",
  highestTimestampSec = 1065,
  highestSentTimestampSec = 1000,
  pendingSamples = 12,
  samplesSent = 500,
  samplesFailed = 0,
  samplesRetried = 0,
  bytesSent = 1536,
  currentShards = 2,
  desiredShards = 2,
  minShards = 1,
  maxShards = 10,
  shardCapacity = 2500,
}: {
  remoteName?: string;
  url?: string;
  highestTimestampSec?: number;
  highestSentTimestampSec?: number;
  pendingSamples?: number;
  samplesSent?: number;
  samplesFailed?: number;
  samplesRetried?: number;
  bytesSent?: number;
  currentShards?: number;
  desiredShards?: number;
  minShards?: number;
  maxShards?: number;
  shardCapacity?: number;
} = {}): ProtoMetricFamily[] => {
  const label = [
    { name: "remote_name", value: remoteName },
    { name: "url", value: url },
  ];

  return [
    {
      name: "prometheus_remote_storage_queue_highest_timestamp_seconds",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: highestTimestampSec } }],
    },
    {
      name: "prometheus_remote_storage_queue_highest_sent_timestamp_seconds",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: highestSentTimestampSec } }],
    },
    {
      name: "prometheus_remote_storage_samples_pending",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: pendingSamples } }],
    },
    {
      name: "prometheus_remote_storage_shards",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: currentShards } }],
    },
    {
      name: "prometheus_remote_storage_shards_desired",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: desiredShards } }],
    },
    {
      name: "prometheus_remote_storage_shards_min",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: minShards } }],
    },
    {
      name: "prometheus_remote_storage_shards_max",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: maxShards } }],
    },
    {
      name: "prometheus_remote_storage_shard_capacity",
      help: "",
      type: "GAUGE",
      metric: [{ label, gauge: { value: shardCapacity } }],
    },
    {
      name: "prometheus_remote_storage_samples_total",
      help: "",
      type: "COUNTER",
      metric: [{ label, counter: { value: samplesSent } }],
    },
    {
      name: "prometheus_remote_storage_samples_failed_total",
      help: "",
      type: "COUNTER",
      metric: [{ label, counter: { value: samplesFailed } }],
    },
    {
      name: "prometheus_remote_storage_samples_retried_total",
      help: "",
      type: "COUNTER",
      metric: [{ label, counter: { value: samplesRetried } }],
    },
    {
      name: "prometheus_remote_storage_bytes_total",
      help: "",
      type: "COUNTER",
      metric: [{ label, counter: { value: bytesSent } }],
    },
  ];
};

describe("RemoteWriteStatusPage", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = new QueryClient();
    vi.stubGlobal(
      "matchMedia",
      vi.fn().mockReturnValue({
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

  const renderPage = () =>
    render(
      <MantineProvider>
        <QueryClientProvider client={client}>
          <RemoteWriteStatusPage />
        </QueryClientProvider>
      </MantineProvider>,
    );

  it("shows empty state when no remote write queues are present", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ status: "success", data: [] }),
      }),
    );

    renderPage();
    expect(screen.getByText("Loading...")).toBeInTheDocument();

    expect(
      await screen.findByText("No remote write endpoints configured"),
    ).toBeInTheDocument();
  });

  it("shows a full-page error alert when the initial fetch fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new Error("backend unavailable")),
    );

    renderPage();

    expect(
      await screen.findByText("Error fetching remote write metrics"),
    ).toBeInTheDocument();
    expect(screen.getByText("backend unavailable")).toBeInTheDocument();
  });

  it("preserves existing queue data and shows a warning alert when a follow-up fetch fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies(),
        }),
      })
      .mockRejectedValueOnce(new Error("poll timeout"));
    vi.stubGlobal("fetch", fetchMock);

    renderPage();

    expect(await screen.findByText("rw-primary")).toBeInTheDocument();
    expect(
      screen.getByText("http://remote.example/api/v1/write"),
    ).toBeInTheDocument();
    expect(screen.getByText("1m 5s")).toBeInTheDocument();
    expect(screen.getByText("1.5 KB")).toBeInTheDocument();

    await client.refetchQueries();

    expect(
      await screen.findByText("Error refreshing remote write metrics"),
    ).toBeInTheDocument();
    expect(screen.getByText("poll timeout")).toBeInTheDocument();
    expect(screen.getByText("rw-primary")).toBeInTheDocument();
  });

  it("computes health status across polls and resets baseline on counter reset", async () => {
    let now = 10_000;
    vi.spyOn(Date, "now").mockImplementation(() => now);

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies({ samplesSent: 100, samplesFailed: 0 }),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies({ samplesSent: 200, samplesFailed: 0 }),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies({ samplesSent: 250, samplesFailed: 5 }),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies({ samplesSent: 300, samplesFailed: 5 }),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: "success",
          data: makeQueueFamilies({ samplesSent: 10, samplesFailed: 0 }),
        }),
      });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();

    expect(await screen.findByText("Calculating...")).toBeInTheDocument();

    now = 20_000;
    await client.refetchQueries();
    await waitFor(() =>
      expect(screen.getByText("Healthy")).toBeInTheDocument(),
    );

    now = 30_000;
    await client.refetchQueries();
    await waitFor(() =>
      expect(screen.getByText("Degraded")).toBeInTheDocument(),
    );

    now = 40_000;
    await client.refetchQueries();
    await waitFor(() =>
      expect(screen.getByText("Healthy")).toBeInTheDocument(),
    );

    now = 50_000;
    await client.refetchQueries();
    await waitFor(() =>
      expect(screen.getByText("Calculating...")).toBeInTheDocument(),
    );
  });
});
