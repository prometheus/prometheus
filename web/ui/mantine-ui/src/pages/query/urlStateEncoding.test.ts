import {
  parseTime,
  formatTime,
  decodePanelOptionsFromURLParams,
  encodePanelOptionsToURLParams,
} from "./urlStateEncoding";
import { GraphDisplayMode, Panel } from "../../state/queryPageSlice";

describe("parseTime", () => {
  test("parses ISO date string correctly", () => {
    expect(parseTime("2024-01-15 12:30:45")).toBe(1705321845000);
  });

  test("parses date-only string correctly", () => {
    expect(parseTime("2024-01-01 00:00:00")).toBe(1704067200000);
  });

  test("parses date with different time values", () => {
    expect(parseTime("2024-06-15 23:59:59")).toBe(1718495999000);
  });
});

describe("formatTime", () => {
  test("formats timestamp to expected string format", () => {
    expect(formatTime(1705321845000)).toBe("2024-01-15 12:30:45");
  });

  test("formats midnight correctly", () => {
    expect(formatTime(1704067200000)).toBe("2024-01-01 00:00:00");
  });

  test("formats end of day correctly", () => {
    expect(formatTime(1718495999000)).toBe("2024-06-15 23:59:59");
  });
});

describe("parseTime and formatTime roundtrip", () => {
  test("roundtrip preserves time", () => {
    const original = "2024-03-20 15:45:30";
    const timestamp = parseTime(original);
    expect(formatTime(timestamp)).toBe(original);
  });
});

describe("decodePanelOptionsFromURLParams", () => {
  test("returns empty array for empty query string", () => {
    expect(decodePanelOptionsFromURLParams("")).toEqual([]);
  });

  test("returns empty array when no expr parameter exists", () => {
    expect(decodePanelOptionsFromURLParams("?foo=bar")).toEqual([]);
  });

  test("decodes single panel with expr only", () => {
    const panels = decodePanelOptionsFromURLParams("g0.expr=up");
    expect(panels).toHaveLength(1);
    expect(panels[0].expr).toBe("up");
  });

  test("decodes URL-encoded expression", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=rate(http_requests_total%5B5m%5D)"
    );
    expect(panels).toHaveLength(1);
    expect(panels[0].expr).toBe("rate(http_requests_total[5m])");
  });

  test("decodes multiple panels", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=up&g1.expr=node_cpu_seconds_total"
    );
    expect(panels).toHaveLength(2);
    expect(panels[0].expr).toBe("up");
    expect(panels[1].expr).toBe("node_cpu_seconds_total");
  });

  test("decodes show_tree parameter", () => {
    const panelsWithTree = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.show_tree=1"
    );
    expect(panelsWithTree[0].showTree).toBe(true);

    const panelsWithoutTree = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.show_tree=0"
    );
    expect(panelsWithoutTree[0].showTree).toBe(false);
  });

  describe("tab parameter", () => {
    test("decodes numeric tab value 0 as graph", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.tab=0");
      expect(panels[0].visualizer.activeTab).toBe("graph");
    });

    test("decodes numeric tab value 1 as table", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.tab=1");
      expect(panels[0].visualizer.activeTab).toBe("table");
    });

    test("decodes string tab value graph", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.tab=graph");
      expect(panels[0].visualizer.activeTab).toBe("graph");
    });

    test("decodes string tab value table", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.tab=table");
      expect(panels[0].visualizer.activeTab).toBe("table");
    });

    test("decodes string tab value explain", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.tab=explain"
      );
      expect(panels[0].visualizer.activeTab).toBe("explain");
    });
  });

  describe("display_mode parameter", () => {
    test("decodes lines display mode", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.display_mode=lines"
      );
      expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Lines);
    });

    test("decodes stacked display mode", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.display_mode=stacked"
      );
      expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Stacked);
    });

    test("decodes heatmap display mode", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.display_mode=heatmap"
      );
      expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Heatmap);
    });
  });

  describe("legacy stacked parameter", () => {
    test("decodes stacked=1 as stacked display mode", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.stacked=1");
      expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Stacked);
    });

    test("decodes stacked=0 as lines display mode", () => {
      const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.stacked=0");
      expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Lines);
    });
  });

  test("decodes y_axis_min parameter", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.y_axis_min=10.5"
    );
    expect(panels[0].visualizer.yAxisMin).toBe(10.5);
  });

  test("decodes empty y_axis_min as null", () => {
    const panels = decodePanelOptionsFromURLParams("g0.expr=up&g0.y_axis_min=");
    expect(panels[0].visualizer.yAxisMin).toBeNull();
  });

  test("decodes show_exemplars parameter", () => {
    const panelsWithExemplars = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.show_exemplars=1"
    );
    expect(panelsWithExemplars[0].visualizer.showExemplars).toBe(true);

    const panelsWithoutExemplars = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.show_exemplars=0"
    );
    expect(panelsWithoutExemplars[0].visualizer.showExemplars).toBe(false);
  });

  test("decodes range_input parameter", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.range_input=2h"
    );
    expect(panels[0].visualizer.range).toBe(7200000); // 2 hours in ms
  });

  test("decodes end_input parameter", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.end_input=2024-01-15%2012%3A30%3A45"
    );
    expect(panels[0].visualizer.endTime).toBe(1705321845000);
  });

  test("decodes moment_input parameter", () => {
    const panels = decodePanelOptionsFromURLParams(
      "g0.expr=up&g0.moment_input=2024-01-15%2012%3A30%3A45"
    );
    expect(panels[0].visualizer.endTime).toBe(1705321845000);
  });

  describe("legacy step_input parameter", () => {
    test("decodes positive step_input as custom resolution", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.step_input=15"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "custom",
        step: 15000,
      });
    });

    test("ignores non-positive step_input", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.step_input=0"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "auto",
        density: "medium",
      });
    });
  });

  describe("resolution parameters", () => {
    test("decodes auto resolution with low density", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.res_type=auto&g0.res_density=low"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "auto",
        density: "low",
      });
    });

    test("decodes auto resolution with medium density", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.res_type=auto&g0.res_density=medium"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "auto",
        density: "medium",
      });
    });

    test("decodes auto resolution with high density", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.res_type=auto&g0.res_density=high"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "auto",
        density: "high",
      });
    });

    test("decodes fixed resolution", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.res_type=fixed&g0.res_step=30"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "fixed",
        step: 30000,
      });
    });

    test("decodes custom resolution", () => {
      const panels = decodePanelOptionsFromURLParams(
        "g0.expr=up&g0.res_type=custom&g0.res_step=60"
      );
      expect(panels[0].visualizer.resolution).toEqual({
        type: "custom",
        step: 60000,
      });
    });
  });

  test("decodes complex panel with all parameters", () => {
    const queryString =
      "g0.expr=rate(http_requests_total%5B5m%5D)" +
      "&g0.show_tree=1" +
      "&g0.tab=graph" +
      "&g0.display_mode=stacked" +
      "&g0.y_axis_min=0" +
      "&g0.show_exemplars=1" +
      "&g0.range_input=1h" +
      "&g0.end_input=2024-01-15%2012%3A30%3A45" +
      "&g0.res_type=fixed" +
      "&g0.res_step=15";

    const panels = decodePanelOptionsFromURLParams(queryString);
    expect(panels).toHaveLength(1);
    expect(panels[0].expr).toBe("rate(http_requests_total[5m])");
    expect(panels[0].showTree).toBe(true);
    expect(panels[0].visualizer.activeTab).toBe("graph");
    expect(panels[0].visualizer.displayMode).toBe(GraphDisplayMode.Stacked);
    expect(panels[0].visualizer.yAxisMin).toBe(0);
    expect(panels[0].visualizer.showExemplars).toBe(true);
    expect(panels[0].visualizer.range).toBe(3600000);
    expect(panels[0].visualizer.endTime).toBe(1705321845000);
    expect(panels[0].visualizer.resolution).toEqual({
      type: "fixed",
      step: 15000,
    });
  });
});

describe("encodePanelOptionsToURLParams", () => {
  const createPanel = (overrides: Partial<Panel> = {}): Panel => ({
    id: "test-id",
    expr: "up",
    showTree: false,
    showMetricsExplorer: false,
    visualizer: {
      activeTab: "table",
      endTime: null,
      range: 3600000,
      resolution: { type: "auto", density: "medium" },
      displayMode: GraphDisplayMode.Lines,
      showExemplars: false,
      yAxisMin: null,
    },
    ...overrides,
  });

  test("encodes single panel with basic settings", () => {
    const panel = createPanel();
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.get("g0.expr")).toBe("up");
    expect(params.get("g0.show_tree")).toBe("0");
    expect(params.get("g0.tab")).toBe("table");
    expect(params.get("g0.range_input")).toBe("1h");
    expect(params.get("g0.display_mode")).toBe("lines");
    expect(params.get("g0.show_exemplars")).toBe("0");
  });

  test("encodes multiple panels", () => {
    const panel1 = createPanel({ expr: "up" });
    const panel2 = createPanel({ expr: "node_cpu_seconds_total" });
    const params = encodePanelOptionsToURLParams([panel1, panel2]);

    expect(params.get("g0.expr")).toBe("up");
    expect(params.get("g1.expr")).toBe("node_cpu_seconds_total");
  });

  test("encodes show_tree as 1 when true", () => {
    const panel = createPanel({ showTree: true });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.get("g0.show_tree")).toBe("1");
  });

  test("encodes different tab values", () => {
    const graphPanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        activeTab: "graph",
      },
    });
    const tablePanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        activeTab: "table",
      },
    });
    const explainPanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        activeTab: "explain",
      },
    });

    expect(encodePanelOptionsToURLParams([graphPanel]).get("g0.tab")).toBe(
      "graph"
    );
    expect(encodePanelOptionsToURLParams([tablePanel]).get("g0.tab")).toBe(
      "table"
    );
    expect(encodePanelOptionsToURLParams([explainPanel]).get("g0.tab")).toBe(
      "explain"
    );
  });

  test("encodes endTime when set", () => {
    const panel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        endTime: 1705321845000,
      },
    });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.get("g0.end_input")).toBe("2024-01-15 12:30:45");
    expect(params.get("g0.moment_input")).toBe("2024-01-15 12:30:45");
  });

  test("does not encode endTime when null", () => {
    const panel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        endTime: null,
      },
    });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.has("g0.end_input")).toBe(false);
    expect(params.has("g0.moment_input")).toBe(false);
  });

  test("encodes range_input in Prometheus duration format", () => {
    const panel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        range: 7200000, // 2 hours
      },
    });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.get("g0.range_input")).toBe("2h");
  });

  describe("resolution encoding", () => {
    test("encodes auto resolution with density", () => {
      const panel = createPanel({
        visualizer: {
          ...createPanel().visualizer,
          resolution: { type: "auto", density: "high" },
        },
      });
      const params = encodePanelOptionsToURLParams([panel]);

      expect(params.get("g0.res_type")).toBe("auto");
      expect(params.get("g0.res_density")).toBe("high");
    });

    test("encodes fixed resolution with step", () => {
      const panel = createPanel({
        visualizer: {
          ...createPanel().visualizer,
          resolution: { type: "fixed", step: 30000 },
        },
      });
      const params = encodePanelOptionsToURLParams([panel]);

      expect(params.get("g0.res_type")).toBe("fixed");
      expect(params.get("g0.res_step")).toBe("30");
    });

    test("encodes custom resolution with step", () => {
      const panel = createPanel({
        visualizer: {
          ...createPanel().visualizer,
          resolution: { type: "custom", step: 60000 },
        },
      });
      const params = encodePanelOptionsToURLParams([panel]);

      expect(params.get("g0.res_type")).toBe("custom");
      expect(params.get("g0.res_step")).toBe("60");
    });
  });

  test("encodes display_mode", () => {
    const linesPanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        displayMode: GraphDisplayMode.Lines,
      },
    });
    const stackedPanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        displayMode: GraphDisplayMode.Stacked,
      },
    });
    const heatmapPanel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        displayMode: GraphDisplayMode.Heatmap,
      },
    });

    expect(
      encodePanelOptionsToURLParams([linesPanel]).get("g0.display_mode")
    ).toBe("lines");
    expect(
      encodePanelOptionsToURLParams([stackedPanel]).get("g0.display_mode")
    ).toBe("stacked");
    expect(
      encodePanelOptionsToURLParams([heatmapPanel]).get("g0.display_mode")
    ).toBe("heatmap");
  });

  test("encodes y_axis_min when set", () => {
    const panel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        yAxisMin: 10.5,
      },
    });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.get("g0.y_axis_min")).toBe("10.5");
  });

  test("does not encode y_axis_min when null", () => {
    const panel = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        yAxisMin: null,
      },
    });
    const params = encodePanelOptionsToURLParams([panel]);

    expect(params.has("g0.y_axis_min")).toBe(false);
  });

  test("encodes show_exemplars", () => {
    const panelWithExemplars = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        showExemplars: true,
      },
    });
    const panelWithoutExemplars = createPanel({
      visualizer: {
        ...createPanel().visualizer,
        showExemplars: false,
      },
    });

    expect(
      encodePanelOptionsToURLParams([panelWithExemplars]).get(
        "g0.show_exemplars"
      )
    ).toBe("1");
    expect(
      encodePanelOptionsToURLParams([panelWithoutExemplars]).get(
        "g0.show_exemplars"
      )
    ).toBe("0");
  });

  test("encodes empty panels array", () => {
    const params = encodePanelOptionsToURLParams([]);
    expect(params.toString()).toBe("");
  });
});

describe("encode and decode roundtrip", () => {
  const createPanel = (overrides: Partial<Panel> = {}): Panel => ({
    id: "test-id",
    expr: "up",
    showTree: false,
    showMetricsExplorer: false,
    visualizer: {
      activeTab: "table",
      endTime: null,
      range: 3600000,
      resolution: { type: "auto", density: "medium" },
      displayMode: GraphDisplayMode.Lines,
      showExemplars: false,
      yAxisMin: null,
    },
    ...overrides,
  });

  test("roundtrip preserves basic panel settings", () => {
    const original = createPanel({
      expr: "rate(http_requests_total[5m])",
      showTree: true,
    });
    const encoded = encodePanelOptionsToURLParams([original]);
    const decoded = decodePanelOptionsFromURLParams(encoded.toString());

    expect(decoded).toHaveLength(1);
    expect(decoded[0].expr).toBe(original.expr);
    expect(decoded[0].showTree).toBe(original.showTree);
  });

  test("roundtrip preserves visualizer settings", () => {
    const original = createPanel({
      visualizer: {
        activeTab: "graph",
        endTime: 1705321845000,
        range: 7200000,
        resolution: { type: "fixed", step: 30000 },
        displayMode: GraphDisplayMode.Stacked,
        showExemplars: true,
        yAxisMin: 0,
      },
    });
    const encoded = encodePanelOptionsToURLParams([original]);
    const decoded = decodePanelOptionsFromURLParams(encoded.toString());

    expect(decoded).toHaveLength(1);
    expect(decoded[0].visualizer.activeTab).toBe(original.visualizer.activeTab);
    expect(decoded[0].visualizer.endTime).toBe(original.visualizer.endTime);
    expect(decoded[0].visualizer.range).toBe(original.visualizer.range);
    expect(decoded[0].visualizer.resolution).toEqual(
      original.visualizer.resolution
    );
    expect(decoded[0].visualizer.displayMode).toBe(
      original.visualizer.displayMode
    );
    expect(decoded[0].visualizer.showExemplars).toBe(
      original.visualizer.showExemplars
    );
    expect(decoded[0].visualizer.yAxisMin).toBe(original.visualizer.yAxisMin);
  });

  test("roundtrip preserves multiple panels", () => {
    const panels = [
      createPanel({ expr: "up" }),
      createPanel({ expr: "node_cpu_seconds_total", showTree: true }),
      createPanel({
        expr: "rate(http_requests_total[5m])",
        visualizer: {
          ...createPanel().visualizer,
          activeTab: "graph",
          displayMode: GraphDisplayMode.Heatmap,
        },
      }),
    ];
    const encoded = encodePanelOptionsToURLParams(panels);
    const decoded = decodePanelOptionsFromURLParams(encoded.toString());

    expect(decoded).toHaveLength(3);
    expect(decoded[0].expr).toBe("up");
    expect(decoded[1].expr).toBe("node_cpu_seconds_total");
    expect(decoded[1].showTree).toBe(true);
    expect(decoded[2].expr).toBe("rate(http_requests_total[5m])");
    expect(decoded[2].visualizer.displayMode).toBe(GraphDisplayMode.Heatmap);
  });

  test("roundtrip preserves auto resolution with all densities", () => {
    for (const density of ["low", "medium", "high"] as const) {
      const original = createPanel({
        visualizer: {
          ...createPanel().visualizer,
          resolution: { type: "auto", density },
        },
      });
      const encoded = encodePanelOptionsToURLParams([original]);
      const decoded = decodePanelOptionsFromURLParams(encoded.toString());

      expect(decoded[0].visualizer.resolution).toEqual({
        type: "auto",
        density,
      });
    }
  });
});
