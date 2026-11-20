// Copyright The Prometheus Authors

import { PropsWithChildren } from "react";
import { MantineProvider } from "@mantine/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../../App";
import QueryPage from "./QueryPage";
import TreeNode from "./TreeNode";
import { nodeType } from "../../promql/ast";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { setPanels } from "../../state/queryPageSlice";

const { dispatch } = vi.hoisted(() => ({ dispatch: vi.fn() }));
vi.mock("../../state/settingsSlice", () => ({
  useSettings: () => ({ pathPrefix: "/prometheus", ready: true }),
}));
vi.mock("../../state/hooks", () => ({
  useAppSelector: () => [],
  useAppDispatch: () => dispatch,
}));
vi.mock("./QueryPanel", () => ({ default: () => null }));
let client: QueryClient;
let now: number;
const wrapper = ({ children }: PropsWithChildren) => (
  <MantineProvider env="test">
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  </MantineProvider>
);
beforeEach(() => {
  client = new QueryClient();
  now = 100000;
  vi.spyOn(Date, "now").mockImplementation(() => now);
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
  window.history.replaceState(null, "", "/");
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["Back", "Query link", "same URL"])("handles %s", async (navigation) => {
  window.history.replaceState(null, "", "/prometheus/query");
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => {})));
  render(<App />);
  const header = within(screen.getByRole("banner"));
  const link = header.getByRole("link", { name: "Query" });
  dispatch.mockClear();

  if (navigation === "same URL") {
    const entries = window.history.length;
    fireEvent.click(link);
    expect(window.history.length).toBe(entries);
    expect(dispatch).not.toHaveBeenCalled();
    return;
  }

  window.history.pushState({}, "", "?g0.expr=up&g0.tab=graph");
  if (navigation === "Query link") {
    fireEvent.click(link);
    expect(location.search).toBe("");
    expect(dispatch).toHaveBeenLastCalledWith(
      setPanels([expect.objectContaining({ expr: "" })]),
    );
    dispatch.mockClear();
  }
  window.history.back();
  const expr = navigation === "Back" ? "" : "up";
  const restored = setPanels([expect.objectContaining({ expr })]);
  await waitFor(() => expect(dispatch).toHaveBeenCalledWith(restored));
});

describe("query diagnostics", () => {
  it.each([30, 31])("shows skew only above 30 seconds (%i)", async (delta) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => ({
        ok: true,
        json: async () => ({
          status: "success",
          data: url.includes("/query?")
            ? { resultType: "scalar", result: [100 - delta, "0"] }
            : [],
        }),
      })),
    );
    const { rerender } = render(<QueryPage />, { wrapper });
    await waitFor(() => expect(client.isFetching()).toBe(0));
    if (delta === 30) {
      expect(
        screen.queryByText("Server time is out of sync"),
      ).not.toBeInTheDocument();
    } else {
      expect(
        await screen.findByText("Server time is out of sync"),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Close" }));
      now = 200000;
      rerender(<QueryPage />);
      expect(
        screen.queryByText("Server time is out of sync"),
      ).not.toBeInTheDocument();
      await act(async () => client.refetchQueries());
      expect(
        await screen.findByText("Server time is out of sync"),
      ).toBeInTheDocument();
    }
  });

  it("reports an unexpected server-time result type", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => ({
        ok: true,
        json: async () => ({
          status: "success",
          data: url.includes("/query?")
            ? { resultType: "vector", result: [] }
            : [],
        }),
      })),
    );
    render(<QueryPage />, { wrapper });
    expect(
      await screen.findByText("Unexpected result type from time query"),
    ).toBeInTheDocument();
  });

  it.each<InstantQueryResult>([
    { resultType: "scalar", result: [100, "1"] },
    { resultType: "string", result: [100, "value"] },
    {
      resultType: "vector",
      result: [
        { metric: { __name__: "up", job: "a" }, value: [100, "1"] },
        { metric: { __name__: "up", job: "b" }, value: [100, "1"] },
      ],
    },
    {
      resultType: "matrix",
      result: [
        { metric: { job: "a" }, values: [[100, "1"]] },
        { metric: { job: "a" }, values: [[100, "2"]] },
      ],
    },
  ])("derives tree statistics from $resultType results", async (data) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({
          ok: true,
          json: async () => ({ status: "success", data }),
        }),
    );
    const reportNodeState = vi.fn();
    render(
      <TreeNode
        node={{ type: nodeType.numberLiteral, val: "1" }}
        selectedNode={null}
        setSelectedNode={vi.fn()}
        reverse={false}
        childIdx={0}
        reportNodeState={reportNodeState}
      />,
      { wrapper },
    );
    const isSeries =
      data.resultType === "vector" || data.resultType === "matrix";
    expect(
      await screen.findByText(isSeries ? /2 results/ : /1 result/),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(reportNodeState).toHaveBeenCalledWith(0, "success"),
    );
    expect(screen.queryByText("__name__")).not.toBeInTheDocument();
    if (isSeries) {
      expect(screen.getByText("job").parentElement).toHaveTextContent(
        data.resultType === "vector" ? "job: 2" : "job: 1",
      );
    }
  });

  it("updates connector geometry and clears the opposite border when reversed", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({
          ok: true,
          json: async () => ({
            status: "success",
            data: { resultType: "scalar", result: [100, "1"] },
          }),
        }),
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      top: 100,
      bottom: 120,
    } as DOMRect);
    const parentEl = document.createElement("div");
    const props = {
      node: { type: nodeType.numberLiteral as const, val: "1" },
      selectedNode: null,
      setSelectedNode: vi.fn(),
      reverse: false,
      childIdx: 0,
      parentEl,
    };
    const { container, rerender } = render(<TreeNode {...props} />, {
      wrapper,
    });
    const connector = container.querySelector<HTMLDivElement>(
      '[style*="border-left-style"]',
    )!;
    expect(connector.style.top).toBe("20px");
    expect(connector.style.borderBottomStyle).toBe("solid");
    rerender(<TreeNode {...props} reverse />);
    expect(connector.style.bottom).toBe("20px");
    expect(connector.style.borderTopStyle).toBe("solid");
    expect(connector.style.borderBottomStyle).toBe("");
    expect(connector.style.borderBottomLeftRadius).toBe("");
    rerender(<TreeNode {...props} />);
    expect(connector.style.borderTopStyle).toBe("");
    expect(connector.style.borderTopLeftRadius).toBe("");
    await screen.findByText(/1 result/);
  });
});
