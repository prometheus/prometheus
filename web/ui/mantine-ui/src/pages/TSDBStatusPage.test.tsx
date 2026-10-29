// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
import { MantineProvider } from "@mantine/core";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { useSuspenseAPIQuery } from "../api/api";
import TSDBStatusPage from "./TSDBStatusPage";

dayjs.extend(timezone);

const settings = vi.hoisted(() => ({ useLocalTime: false }));
vi.mock("../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "", ...settings }),
}));
vi.mock("../api/api", async (original) => ({
  ...(await original<typeof import("../api/api")>()),
  useSuspenseAPIQuery: vi.fn(),
}));

const wrapper = ({ children }: PropsWithChildren) => (
  <MantineProvider env="test">{children}</MantineProvider>
);

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
  // The autosized series selector listens for font loading, which jsdom lacks.
  Object.defineProperty(document, "fonts", {
    configurable: true,
    value: { addEventListener: vi.fn(), removeEventListener: vi.fn() },
  });
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
  vi.mocked(useSuspenseAPIQuery).mockImplementation((({
    path,
  }: {
    path: string;
  }) => ({
    data: {
      data:
        path === "/features"
          ? { api: { admin: true } }
          : {
              headStats: {
                numSeries: 1,
                numLabelPairs: 2,
                chunkCount: 1,
                minTime: Date.UTC(2026, 9, 29, 10),
                maxTime: Date.UTC(2026, 9, 29, 12),
              },
              seriesCountByMetricName: [{ name: "up", value: 1 }],
              labelValueCountByLabelName: [{ name: "job", value: 1 }],
              memoryInBytesByLabelName: [{ name: "job", value: 40 }],
              seriesCountByLabelValuePair: [
                { name: "job=prometheus", value: 1 },
              ],
            },
    },
  })) as never);
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
  settings.useLocalTime = false;
  Reflect.deleteProperty(Element.prototype, "scrollIntoView");
  Reflect.deleteProperty(document, "fonts");
});

describe("TSDBStatusPage", () => {
  it.each([
    { picker: "Start time", param: "start", month: 9, day: 20 },
    { picker: "End time", param: "end", month: 9, day: 20 },
    {
      picker: "Start time",
      param: "start",
      month: 2,
      day: 29,
      hour: 2,
      minute: 30,
    },
    {
      picker: "End time",
      param: "end",
      month: 2,
      day: 29,
      hour: 2,
      minute: 30,
    },
    {
      picker: "Start time",
      param: "start",
      month: 2,
      day: 29,
      hour: 3,
      minute: 30,
      useLocalTime: true,
    },
  ])(
    "sends the $param time shown in its picker across a DST change ($month/$day, local: $useLocalTime)",
    async ({
      picker,
      param,
      month,
      day,
      hour = 0,
      minute = 0,
      useLocalTime = false,
    }) => {
      vi.stubEnv("TZ", "Europe/Berlin");
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(Date.UTC(2026, month, 29, 12));
      settings.useLocalTime = useLocalTime;
      render(<TSDBStatusPage />, { wrapper });
      fireEvent.change(
        screen.getByRole("textbox", { name: "Series selector(s)" }),
        { target: { value: 'up{job="prometheus"}' } },
      );
      const input = screen.getByRole("button", { name: picker });
      fireEvent.click(input);
      fireEvent.click(
        screen.getByRole("button", {
          name: new Date(Date.UTC(2026, month, day)).toLocaleDateString(
            "en-GB",
            {
              day: "numeric",
              month: "long",
              year: "numeric",
              timeZone: "UTC",
            },
          ),
        }),
      );
      if (hour !== 0 || minute !== 0) {
        fireEvent.change(screen.getByRole("spinbutton", { name: "Hours" }), {
          target: { value: String(hour) },
        });
        fireEvent.change(screen.getByRole("spinbutton", { name: "Minutes" }), {
          target: { value: String(minute) },
        });
      }
      const displayed = new Date(Date.UTC(2026, month, day, hour, minute))
        .toISOString()
        .slice(0, 19)
        .replace("T", " ");
      expect(input).toHaveTextContent(displayed);
      const expected = useLocalTime
        ? new Date(2026, month, day, hour, minute).getTime()
        : Date.UTC(2026, month, day, hour, minute);

      fireEvent.click(screen.getByRole("button", { name: "Delete series" }));
      expect(
        await screen.findByText(/^Successfully deleted series/),
      ).toBeInTheDocument();
      expect(fetch).toHaveBeenCalledOnce();
      const [url] = vi.mocked(fetch).mock.calls[0];
      expect(
        new URL(String(url), "http://localhost").searchParams.get(param),
      ).toBe(String(expected / 1000));
    },
  );
});
