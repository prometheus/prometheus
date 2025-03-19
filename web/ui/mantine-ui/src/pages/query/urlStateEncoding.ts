import {
  GraphDisplayMode,
  Panel,
  newDefaultPanel,
} from "../../state/queryPageSlice";
import dayjs from "dayjs";
import {
  formatPrometheusDuration,
  parsePrometheusDuration,
} from "../../lib/formatTime";

export function parseTime(timeText: string): number {
  return dayjs.utc(timeText).valueOf();
}

export const decodePanelOptionsFromURLParams = (query: string): Panel[] => {
  const urlParams = new URLSearchParams(query);
  const panels = [];

  for (let i = 0; ; i++) {
    if (!urlParams.has(`g${i}.expr`)) {
      // Every panel should have an expr, so if we don't find one, we're done.
      break;
    }

    const panel = newDefaultPanel();

    const decodeSetting = (setting: string, fn: (_value: string) => void) => {
      const param = `g${i}.${setting}`;
      if (urlParams.has(param)) {
        fn(urlParams.get(param) as string);
      }
    };

    decodeSetting("expr", (value) => {
      panel.expr = value;
    });
    decodeSetting("show_tree", (value) => {
      panel.showTree = value === "1";
    });
    decodeSetting("tab", (value) => {
      // Numeric values are deprecated (from the old UI), but we still support decoding them.
      switch (value) {
        case "0":
        case "graph":
          panel.visualizer.activeTab = "graph";
          break;
        case "1":
        case "table":
          panel.visualizer.activeTab = "table";
          break;
        case "explain":
          panel.visualizer.activeTab = "explain";
          break;
        default:
          console.log("Unknown tab", value);
      }
    });
    decodeSetting("display_mode", (value) => {
      panel.visualizer.displayMode = value as GraphDisplayMode;
    });
    decodeSetting("stacked", (value) => {
      panel.visualizer.displayMode =
        value === "1" ? GraphDisplayMode.Stacked : GraphDisplayMode.Lines;
    });
    decodeSetting("show_exemplars", (value) => {
      panel.visualizer.showExemplars = value === "1";
    });
    decodeSetting("range_input", (value) => {
      panel.visualizer.range =
        parsePrometheusDuration(value) || panel.visualizer.range;
    });
    decodeSetting("end_input", (value) => {
      panel.visualizer.endTime = parseTime(value);
    });
    // Legacy "step_input" parameter, overridden below by
    // "res_type" / "res_density" / "res_step" if present.
    decodeSetting("step_input", (value) => {
      if (parseInt(value) > 0) {
        panel.visualizer.resolution = {
          type: "custom",
          step: parseInt(value) * 1000,
        };
      }
    });
    decodeSetting("res_type", (value) => {
      switch (value) {
        case "auto":
          decodeSetting("res_density", (density) => {
            if (["low", "medium", "high"].includes(density)) {
              panel.visualizer.resolution = {
                type: "auto",
                density: density as "low" | "medium" | "high",
              };
            }
          });
          break;
        case "fixed":
          decodeSetting("res_step", (step) => {
            panel.visualizer.resolution = {
              type: "fixed",
              step: parseFloat(step) * 1000,
            };
          });
          break;
        case "custom":
          decodeSetting("res_step", (step) => {
            panel.visualizer.resolution = {
              type: "custom",
              step: parseFloat(step) * 1000,
            };
          });
          break;
        default:
          console.log("Unknown resolution type", value);
      }
    });
    decodeSetting("moment_input", (value) => {
      panel.visualizer.endTime = parseTime(value);
    });

    panels.push(panel);
  }

  return panels;
};

export function formatTime(time: number): string {
  return dayjs.utc(time).format("YYYY-MM-DD HH:mm:ss");
}

export const encodePanelOptionsToURLParams = (
  panels: Panel[]
): URLSearchParams => {
  const params = new URLSearchParams();

  const addParam = (idx: number, param: string, value: string) =>
    params.append(`g${idx}.${param}`, value);

  panels.forEach((p, idx) => {
    addParam(idx, "expr", p.expr);
    addParam(idx, "show_tree", p.showTree ? "1" : "0");
    addParam(idx, "tab", p.visualizer.activeTab);
    if (p.visualizer.endTime !== null) {
      addParam(idx, "end_input", formatTime(p.visualizer.endTime));
      addParam(idx, "moment_input", formatTime(p.visualizer.endTime));
    }
    addParam(idx, "range_input", formatPrometheusDuration(p.visualizer.range));

    switch (p.visualizer.resolution.type) {
      case "auto":
        addParam(idx, "res_type", "auto");
        addParam(idx, "res_density", p.visualizer.resolution.density);
        break;
      case "fixed":
        addParam(idx, "res_type", "fixed");
        addParam(
          idx,
          "res_step",
          (p.visualizer.resolution.step / 1000).toString()
        );
        break;
      case "custom":
        addParam(idx, "res_type", "custom");
        addParam(
          idx,
          "res_step",
          (p.visualizer.resolution.step / 1000).toString()
        );
        break;
    }

    addParam(idx, "display_mode", p.visualizer.displayMode);
    addParam(idx, "show_exemplars", p.visualizer.showExemplars ? "1" : "0");
  });

  return params;
};
