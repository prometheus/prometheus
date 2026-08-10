// Copyright The Prometheus Authors

import { MantineProvider } from "@mantine/core";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { useSuspenseAPIQuery } from "../api/api";
import ScrapePoolConfig from "./ScrapePoolConfig";

vi.mock("../api/api", () => ({
  useSuspenseAPIQuery: vi.fn(),
}));

vi.mock("@mantine/code-highlight", () => ({
  CodeHighlight: ({ code }: { code: string }) => (
    <pre data-testid="scrape-config-yaml">{code}</pre>
  ),
}));

describe("ScrapePoolConfig", () => {
  beforeEach(() => {
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
    vi.mocked(useSuspenseAPIQuery).mockReturnValue({
      data: {
        data: {
          yaml: "job_name: imported-job\nscrape_interval: 30s\n",
        },
      },
    } as never);
  });

  it("loads the selected pool only when expanded", () => {
    render(
      <MantineProvider>
        <MemoryRouter>
          <ScrapePoolConfig pool="imported-job" />
        </MemoryRouter>
      </MantineProvider>,
    );

    expect(useSuspenseAPIQuery).not.toHaveBeenCalled();

    const control = screen.getByRole("button", {
      name: "Scrape configuration",
    });
    fireEvent.click(control);

    expect(useSuspenseAPIQuery).toHaveBeenCalledWith({
      path: "/scrape_pools/config",
      params: { scrapePool: "imported-job" },
    });
    expect(screen.getByTestId("scrape-config-yaml")).toHaveTextContent(
      "job_name: imported-job",
    );

    fireEvent.click(control);
    expect(screen.queryByTestId("scrape-config-yaml")).not.toBeInTheDocument();
  });
});
