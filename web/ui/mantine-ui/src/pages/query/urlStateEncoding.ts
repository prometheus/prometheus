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
    decodeSetting("tab", (value) => {
      panel.visualizer.activeTab = value === "0" ? "graph" : "table";
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
    decodeSetting("moment_input", (value) => {
      panel.visualizer.endTime = parseTime(value);
    });
    decodeSetting("step_input", (value) => {
      if (parseInt(value) > 0) {
        panel.visualizer.resolution = {
          type: "custom",
          value: parseInt(value) * 1000,
        };
      }
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
    addParam(idx, "tab", p.visualizer.activeTab === "graph" ? "0" : "1");
    if (p.visualizer.endTime !== null) {
      addParam(idx, "end_input", formatTime(p.visualizer.endTime));
      addParam(idx, "moment_input", formatTime(p.visualizer.endTime));
    }
    addParam(idx, "range_input", formatPrometheusDuration(p.visualizer.range));
    // TODO: Support the other new resolution types.
    if (p.visualizer.resolution.type === "custom") {
      addParam(
        idx,
        "step_input",
        (p.visualizer.resolution.value / 1000).toString()
      );
    }
    addParam(idx, "display_mode", p.visualizer.displayMode);
    addParam(idx, "show_exemplars", p.visualizer.showExemplars ? "1" : "0");
  });

  return params;
};
