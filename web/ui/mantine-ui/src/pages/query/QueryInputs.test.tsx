// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
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

vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({
    pathPrefix: "/prometheus",
    enableAutocomplete: false,
    enableLinter: false,
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
});

describe("query input drafts", () => {
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
