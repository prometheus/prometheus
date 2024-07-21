import { randomId } from "@mantine/hooks";
import { PayloadAction, createSlice } from "@reduxjs/toolkit";

export enum GraphDisplayMode {
  Lines = "lines",
  Stacked = "stacked",
  Heatmap = "heatmap",
}

export type GraphResolution =
  | {
      type: "auto";
      density: "low" | "medium" | "high";
    }
  | {
      type: "fixed";
      value: number; // Resolution step in milliseconds.
    }
  | {
      type: "custom";
      value: number; // Resolution step in milliseconds.
    };

export const getEffectiveResolution = (
  resolution: GraphResolution,
  range: number
) => {
  switch (resolution.type) {
    case "auto": {
      const factor =
        resolution.density === "high"
          ? 750
          : resolution.density === "medium"
            ? 250
            : 100;
      return Math.max(Math.floor(range / factor), 1);
    }
    case "fixed":
      return resolution.value; // TODO: Scope this to a list?
    case "custom":
      return resolution.value;
  }
};

// NOTE: This is not represented as a discriminated union type
// because we want to preserve and partially share settings while
// switching between display modes.
export interface Visualizer {
  activeTab: "table" | "graph" | "explain";
  endTime: number | null; // Timestamp in milliseconds.
  range: number; // Range in milliseconds.
  resolution: GraphResolution;
  displayMode: GraphDisplayMode;
  showExemplars: boolean;
}

export type Panel = {
  // The id is helpful as a stable key for React.
  id: string;
  expr: string;
  exprStale: boolean;
  showMetricsExplorer: boolean;
  visualizer: Visualizer;
};

interface QueryPageState {
  panels: Panel[];
}

const newDefaultPanel = (): Panel => ({
  id: randomId(),
  expr: "",
  exprStale: false,
  showMetricsExplorer: false,
  visualizer: {
    activeTab: "table",
    endTime: null,
    range: 3600 * 1000,
    resolution: { type: "auto", density: "medium" },
    displayMode: GraphDisplayMode.Lines,
    showExemplars: false,
  },
});

const initialState: QueryPageState = {
  panels: [newDefaultPanel()],
};

export const queryPageSlice = createSlice({
  name: "queryPage",
  initialState,
  reducers: {
    addPanel: (state) => {
      state.panels.push(newDefaultPanel());
    },
    removePanel: (state, { payload }: PayloadAction<number>) => {
      state.panels.splice(payload, 1);
    },
    setExpr: (
      state,
      { payload }: PayloadAction<{ idx: number; expr: string }>
    ) => {
      state.panels[payload.idx].expr = payload.expr;
    },
    setVisualizer: (
      state,
      { payload }: PayloadAction<{ idx: number; visualizer: Visualizer }>
    ) => {
      state.panels[payload.idx].visualizer = payload.visualizer;
    },
  },
});

export const { addPanel, removePanel, setExpr, setVisualizer } =
  queryPageSlice.actions;

export default queryPageSlice.reducer;
