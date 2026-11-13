// Copyright The Prometheus Authors

import { PropsWithChildren, StrictMode, useState } from "react";
import { MantineProvider } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAPIQuery } from "../../api/api";
import ExpressionInput from "./ExpressionInput";
import RangeInput from "./RangeInput";
import ResolutionInput from "./ResolutionInput";
import TimeInput from "./TimeInput";
import LabelsExplorer from "./MetricsExplorer/LabelsExplorer";
import { GraphResolution } from "../../state/queryPageSlice";

const settings = vi.hoisted(() => ({ useLocalTime: false }));
vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({
    pathPrefix: "/prometheus",
    enableAutocomplete: false,
    enableLinter: false,
    ...settings,
  }),
}));
vi.mock("../../state/hooks", () => ({
  useAppSelector: () => ({ queryHistory: [] }),
}));
vi.mock("./MetricsExplorer/MetricsExplorer", () => ({ default: () => null }));
vi.mock("@mantine/notifications", () => ({ notifications: { show: vi.fn() } }));
vi.mock("@uiw/react-codemirror", async (original) => ({
  ...(await original<typeof import("@uiw/react-codemirror")>()),
  default: ({
    value,
    onChange,
  }: {
    value: string;
    onChange: (value: string) => void;
  }) => (
    <textarea
      aria-label="Expression"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

let client: QueryClient;
const wrapper = ({ children }: PropsWithChildren) => (
  <MantineProvider env="test">
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  </MantineProvider>
);
const props = {
  initialExpr: "up+1",
  metricNames: [],
  executeQuery: vi.fn(),
  treeShown: false,
  setShowTree: vi.fn(),
  duplicatePanel: vi.fn(),
  removePanel: vi.fn(),
};
const pending: {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
}[] = [];
async function format() {
  await waitFor(() =>
    expect(screen.queryByRole("menu")).not.toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole("button", { name: "Show query options" }));
  fireEvent.click(
    await screen.findByRole("menuitem", { name: "Format expression" }),
  );
}
async function reply(value: string) {
  await act(async () =>
    pending[pending.length - 1].resolve({
      ok: true,
      json: async () => ({ status: "success", data: value }),
    }),
  );
}

beforeEach(() => {
  client = new QueryClient();
  Element.prototype.scrollIntoView = vi.fn();
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
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () => new Promise((resolve, reject) => pending.push({ resolve, reject })),
    ),
  );
});
afterEach(() => {
  cleanup();
  client.clear();
  pending.length = 0;
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
  settings.useLocalTime = false;
  Reflect.deleteProperty(Element.prototype, "scrollIntoView");
});

describe("query input drafts", () => {
  it.each([
    {
      timezone: "Europe/Berlin",
      useLocalTime: false,
      currentDay: 29,
      selectedDays: [20, 21],
      displayedHour: 12,
      queryHour: 12,
    },
    {
      timezone: "Europe/Berlin",
      useLocalTime: false,
      currentDay: 20,
      selectedDays: [28, 29],
      displayedHour: 12,
      queryHour: 12,
    },
    {
      timezone: "UTC",
      useLocalTime: false,
      currentDay: 29,
      selectedDays: [20, 21],
      displayedHour: 12,
      queryHour: 12,
    },
    {
      timezone: "Europe/Berlin",
      useLocalTime: true,
      currentDay: 29,
      selectedDays: [20, 21],
      displayedHour: 13,
      queryHour: 11,
    },
    {
      timezone: "Europe/Berlin",
      useLocalTime: false,
      month: 2,
      currentDay: 29,
      initialHour: 2,
      minute: 30,
      selectedDays: [28, 29, 30],
      displayedHour: 2,
      queryHour: 2,
    },
    {
      timezone: "America/New_York",
      useLocalTime: false,
      month: 2,
      currentDay: 7,
      initialHour: 2,
      minute: 30,
      selectedDays: [8, 9],
      displayedHour: 2,
      queryHour: 2,
    },
    {
      timezone: "Australia/Lord_Howe",
      useLocalTime: false,
      currentDay: 4,
      initialHour: 2,
      minute: 15,
      selectedDays: [3, 4, 5],
      displayedHour: 2,
      queryHour: 2,
    },
    {
      timezone: "Asia/Beirut",
      useLocalTime: false,
      month: 2,
      currentDay: 29,
      initialHour: 0,
      minute: 30,
      selectedDays: [28, 29, 30],
      displayedHour: 0,
      queryHour: 0,
    },
  ])(
    "preserves the hour across date edits in $timezone (local time: $useLocalTime, current day: $currentDay)",
    ({
      timezone,
      useLocalTime,
      currentDay,
      selectedDays,
      displayedHour,
      queryHour,
      month = 9,
      initialHour = 12,
      minute = 0,
    }) => {
      vi.stubEnv("TZ", timezone);
      vi.useFakeTimers({ toFake: ["Date"] });
      const initialTime = Date.UTC(
        2026,
        month,
        currentDay,
        initialHour,
        minute,
      );
      vi.setSystemTime(initialTime);
      settings.useLocalTime = useLocalTime;
      const onChangeTime = vi.fn();
      const timeProps = {
        range: 3600000,
        description: "Evaluation time",
        onChangeTime,
      };
      const { rerender } = render(
        <TimeInput {...timeProps} time={initialTime} />,
        { wrapper },
      );
      const picker = screen.getByRole("button", { name: "Evaluation time" });
      const display = (day: number) =>
        new Date(Date.UTC(2026, month, day, displayedHour, minute))
          .toISOString()
          .slice(0, 19)
          .replace("T", " ");
      expect(picker).toHaveTextContent(display(currentDay));
      fireEvent.click(picker);

      for (const day of selectedDays) {
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
        const expectedTime = Date.UTC(2026, month, day, queryHour, minute);
        expect(onChangeTime).toHaveBeenLastCalledWith(expectedTime);
        rerender(<TimeInput {...timeProps} time={expectedTime} />);
        expect(picker).toHaveTextContent(display(day));
      }
    },
  );

  it.each([
    { field: "Hours", close: "Enter" },
    { field: "Minutes", close: "Escape", partial: true },
    { field: "Seconds", close: "Apply time" },
    { field: "Hours", close: "Toggle", partial: true },
    { field: "Hours", close: "Outside" },
  ])(
    "restores a cleared $field field after closing with $close and reopening",
    async ({ field, close, partial = false }) => {
      vi.stubEnv("TZ", "UTC");
      const onChangeTime = vi.fn();
      render(
        <StrictMode>
          <TimeInput
            time={Date.UTC(2026, 9, 29, 12, 34, 56)}
            range={3600000}
            description="Evaluation time"
            onChangeTime={onChangeTime}
          />
        </StrictMode>,
        {
          // Keep fields mounted to cover reopening during the closing transition.
          wrapper: ({ children }) => (
            <MantineProvider
              env="test"
              theme={{
                components: {
                  Popover: { defaultProps: { keepMounted: true } },
                },
              }}
            >
              {children}
            </MantineProvider>
          ),
        },
      );
      const picker = screen.getByRole("button", { name: "Evaluation time" });
      fireEvent.click(picker);
      const input = await screen.findByRole("spinbutton", { name: field });
      fireEvent.keyDown(input, { key: "Delete" });
      expect(input).toHaveValue("");
      if (partial) {
        fireEvent.change(screen.getByRole("spinbutton", { name: "Hours" }), {
          target: { value: "09" },
        });
      }

      if (close === "Apply time") {
        fireEvent.click(screen.getByRole("button", { name: close }));
      } else if (close === "Toggle") {
        fireEvent.click(picker);
      } else if (close === "Outside") {
        fireEvent.mouseDown(document.body);
      } else {
        fireEvent.keyDown(input, { key: close });
      }
      await waitFor(() =>
        expect(picker).toHaveAttribute("aria-expanded", "false"),
      );
      fireEvent.click(picker);

      expect(picker).toHaveTextContent("2026-10-29 12:34:56");
      for (const [name, value] of [
        ["Hours", "12"],
        ["Minutes", "34"],
        ["Seconds", "56"],
      ]) {
        expect(await screen.findByRole("spinbutton", { name })).toHaveValue(
          value,
        );
      }
      expect(onChangeTime).not.toHaveBeenCalled();
    },
  );

  it.each([1, 3])(
    "synchronizes a local DST-gap edit starting at %i:30",
    (initialHour) => {
      vi.stubEnv("TZ", "Europe/Berlin");
      settings.useLocalTime = true;
      const onChangeTime = vi.fn();
      const initialTime = new Date(2026, 2, 29, initialHour, 30).getTime();
      const normalizedTime = Date.parse("2026-03-29T01:30:00Z");
      function ControlledTimeInput() {
        const [time, setTime] = useState<number | null>(initialTime);
        return (
          <TimeInput
            time={time}
            range={3600000}
            description="Evaluation time"
            onChangeTime={(value) => {
              onChangeTime(value);
              setTime(value);
            }}
          />
        );
      }
      render(<ControlledTimeInput />, { wrapper });
      const picker = screen.getByRole("button", { name: "Evaluation time" });
      fireEvent.click(picker);

      for (let edit = 0; edit < 2; edit++) {
        fireEvent.change(screen.getByRole("spinbutton", { name: "Hours" }), {
          target: { value: "02" },
        });
        expect(onChangeTime).toHaveBeenLastCalledWith(normalizedTime);
        expect(picker).toHaveTextContent("2026-03-29 03:30:00");
        expect(screen.getByRole("spinbutton", { name: "Hours" })).toHaveValue(
          "03",
        );
        expect(screen.getByRole("spinbutton", { name: "Minutes" })).toHaveValue(
          "30",
        );
      }

      fireEvent.click(screen.getByRole("button", { name: "30 March 2026" }));
      expect(onChangeTime).toHaveBeenLastCalledWith(
        Date.parse("2026-03-30T01:30:00Z"),
      );
      expect(picker).toHaveTextContent("2026-03-30 03:30:00");
      expect(screen.getByRole("spinbutton", { name: "Hours" })).toHaveValue(
        "03",
      );
    },
  );

  it.each([
    { timezone: "Europe/Berlin", date: "2026-03-29", hour: 2, minute: 30 },
    { timezone: "America/New_York", date: "2026-03-08", hour: 2, minute: 30 },
    {
      timezone: "Australia/Lord_Howe",
      date: "2026-10-04",
      hour: 2,
      minute: 15,
    },
    { timezone: "Asia/Beirut", date: "2026-03-29", hour: 0, minute: 30 },
  ])(
    "edits and clears a UTC time inside a local DST gap in $timezone",
    ({ timezone, date, hour, minute }) => {
      vi.stubEnv("TZ", timezone);
      const onChangeTime = vi.fn();
      const props = {
        range: 3600000,
        description: "Evaluation time",
        onChangeTime,
      };
      let time = Date.parse(`${date}T01:00:00Z`);
      const { rerender } = render(<TimeInput {...props} time={time} />, {
        wrapper,
      });
      const picker = screen.getByRole("button", { name: "Evaluation time" });
      fireEvent.click(picker);

      for (const [field, value, offset] of [
        ["Hours", hour, 3600000],
        ["Minutes", minute, 60000],
        ["Seconds", 17, 1000],
      ] as const) {
        fireEvent.change(screen.getByRole("spinbutton", { name: field }), {
          target: { value: String(value) },
        });
        time =
          field === "Hours"
            ? Date.parse(`${date}T00:00:00Z`) + value * offset
            : time + value * offset;
        expect(onChangeTime).toHaveBeenLastCalledWith(time);
        rerender(<TimeInput {...props} time={time} />);
      }

      const expected = new Date(time)
        .toISOString()
        .slice(0, 19)
        .replace("T", " ");
      expect(picker).toHaveTextContent(expected);
      fireEvent.click(screen.getByRole("button", { name: "Apply time" }));
      expect(picker).toHaveAttribute("aria-expanded", "false");
      fireEvent.click(picker);
      expect(screen.getByRole("spinbutton", { name: "Hours" })).toHaveValue(
        String(hour).padStart(2, "0"),
      );
      expect(screen.getByRole("spinbutton", { name: "Minutes" })).toHaveValue(
        String(minute).padStart(2, "0"),
      );
      fireEvent.keyDown(screen.getByRole("spinbutton", { name: "Seconds" }), {
        key: "Enter",
      });
      expect(picker).toHaveAttribute("aria-expanded", "false");
      fireEvent.click(screen.getByRole("button", { name: "Clear time" }));
      expect(onChangeTime).toHaveBeenLastCalledWith(null);
      rerender(<TimeInput {...props} time={null} />);
      expect(picker).toHaveTextContent("Evaluation time");
    },
  );

  it.each<{ label: string; resolution: GraphResolution }>([
    { label: "Low res.", resolution: { type: "auto", density: "low" } },
    { label: "30s", resolution: { type: "fixed", step: 30000 } },
    { label: "Enter value...", resolution: { type: "custom", step: 60000 } },
  ])(
    "selects $label and ignores deselection",
    async ({ label, resolution }) => {
      const onChangeResolution = vi.fn();
      const { rerender } = render(
        <ResolutionInput
          resolution={{ type: "fixed", step: 60000 }}
          range={600000}
          onChangeResolution={onChangeResolution}
        />,
        { wrapper },
      );
      fireEvent.click(screen.getByTitle("Resolution"));
      fireEvent.click(await screen.findByRole("option", { name: label }));
      expect(onChangeResolution).toHaveBeenCalledExactlyOnceWith(resolution);

      rerender(
        <ResolutionInput
          resolution={resolution}
          range={600000}
          onChangeResolution={onChangeResolution}
        />,
      );
      onChangeResolution.mockClear();
      fireEvent.click(screen.getByTitle("Resolution"));
      fireEvent.click(await screen.findByRole("option", { name: label }));
      expect(onChangeResolution).not.toHaveBeenCalled();
      expect(screen.getByTitle("Resolution")).toHaveValue(label);
    },
  );

  it("keeps the selected matcher operator when it is deselected", async () => {
    const insertText = vi.fn();
    render(
      <LabelsExplorer
        metricName="up"
        insertText={insertText}
        hideLabelsExplorer={vi.fn()}
      />,
      { wrapper },
    );
    await act(async () =>
      pending[0].resolve({
        ok: true,
        json: async () => ({
          status: "success",
          data: [{ __name__: "up", job: "prometheus" }],
        }),
      }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Filter..." }));
    fireEvent.change(screen.getByPlaceholderText("label value"), {
      target: { value: "prometheus" },
    });
    const operator = screen.getAllByRole("combobox")[0];
    fireEvent.click(operator);
    fireEvent.click(await screen.findByRole("option", { name: "!=" }));
    expect(operator).toHaveValue("!=");
    fireEvent.click(operator);
    fireEvent.click(await screen.findByRole("option", { name: "!=" }));
    expect(operator).toHaveValue("!=");
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    fireEvent.click(screen.getByRole("button", { name: "Insert" }));
    expect(insertText).toHaveBeenCalledExactlyOnceWith('up{job!="prometheus"}');
  });

  it("preserves expression edits and focus until the initial expression changes", () => {
    const { rerender } = render(<ExpressionInput {...props} />, { wrapper });
    const editor = screen.getByRole("textbox", { name: "Expression" });
    editor.focus();
    fireEvent.change(editor, { target: { value: "draft" } });
    rerender(<ExpressionInput {...props} treeShown />);
    expect(editor).toHaveValue("draft");
    expect(editor).toHaveFocus();
    rerender(<ExpressionInput {...props} initialExpr="new" />);
    expect(screen.getByRole("textbox", { name: "Expression" })).toBe(editor);
    expect(editor).toHaveValue("new");
    expect(editor).toHaveFocus();
  });

  it.each([
    { change: "edit", failure: false },
    { change: "prop change", failure: false },
    { change: "unmount", failure: false },
    { change: "edit", failure: true },
    { change: "prop change", failure: true },
    { change: "unmount", failure: true },
  ])(
    "ignores formatting completion after $change (failure: $failure)",
    async ({ change, failure }) => {
      const { rerender, unmount } = render(<ExpressionInput {...props} />, {
        wrapper,
      });
      await format();
      expect(pending).toHaveLength(1);
      if (change === "edit")
        fireEvent.change(screen.getByRole("textbox", { name: "Expression" }), {
          target: { value: "changed" },
        });
      else if (change === "prop change")
        rerender(<ExpressionInput {...props} initialExpr="changed" />);
      else unmount();
      if (failure)
        await act(async () => pending[0].reject(new Error("obsolete error")));
      else await reply("up + 1");
      expect(notifications.show).not.toHaveBeenCalled();
      if (change !== "unmount")
        expect(screen.getByRole("textbox", { name: "Expression" })).toHaveValue(
          "changed",
        );
    },
  );

  it("applies a current formatting result once and reports a cached-result refetch error", async () => {
    const cached = renderHook(
      () =>
        useAPIQuery<string>({
          path: "/format_query",
          params: { query: "up+1" },
          enabled: false,
        }),
      { wrapper },
    );
    render(<ExpressionInput {...props} />, { wrapper });
    await format();
    await reply("up + 1");
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "Expression" })).toHaveValue(
        "up + 1",
      ),
    );
    expect(notifications.show).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(cached.result.current.data?.data).toBe("up + 1"),
    );
    fireEvent.change(screen.getByRole("textbox", { name: "Expression" }), {
      target: { value: "up+1" },
    });
    await format();
    await act(async () =>
      pending[pending.length - 1].reject(new Error("format failed")),
    );
    await waitFor(() => expect(notifications.show).toHaveBeenCalledTimes(2));
    expect(notifications.show).toHaveBeenLastCalledWith(
      expect.objectContaining({
        title: "Error formatting query",
        message: "format failed",
      }),
    );
    expect(screen.getByRole("textbox", { name: "Expression" })).toHaveValue(
      "up+1",
    );
  });

  it("preserves range drafts, resets changed props, and recovers invalid input", () => {
    const onChangeRange = vi.fn();
    const { rerender } = render(
      <RangeInput range={60000} onChangeRange={onChangeRange} />,
      { wrapper },
    );
    const input = screen.getByRole("textbox", { name: "Range" });
    input.focus();
    fireEvent.change(input, { target: { value: "2m" } });
    rerender(<RangeInput range={60000} onChangeRange={onChangeRange} />);
    expect(input).toHaveValue("2m");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChangeRange).toHaveBeenLastCalledWith(120000);
    rerender(<RangeInput range={300000} onChangeRange={onChangeRange} />);
    expect(input).toHaveValue("5m");
    expect(input).toHaveFocus();
    fireEvent.change(input, { target: { value: "invalid" } });
    fireEvent.blur(input);
    expect(input).toHaveValue("5m");
    fireEvent.click(screen.getByRole("button", { name: "Increase range" }));
    expect(onChangeRange).toHaveBeenLastCalledWith(900000);
    fireEvent.click(screen.getByRole("button", { name: "Decrease range" }));
    expect(onChangeRange).toHaveBeenLastCalledWith(60000);
  });
});
