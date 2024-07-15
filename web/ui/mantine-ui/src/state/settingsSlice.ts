import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { useAppSelector } from "./hooks";

interface Settings {
  consolesLink: string | null;
  agentMode: boolean;
  ready: boolean;
  pathPrefix: string;
  useLocalTime: boolean;
  enableQueryHistory: boolean;
  enableAutocomplete: boolean;
  enableSyntaxHighlighting: boolean;
  enableLinter: boolean;
  showAnnotations: boolean;
}

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_CONSOLES_LINK: string;
declare const GLOBAL_AGENT_MODE: string;
declare const GLOBAL_READY: string;

const initialState: Settings = {
  consolesLink:
    GLOBAL_CONSOLES_LINK === "CONSOLES_LINK_PLACEHOLDER" ||
    GLOBAL_CONSOLES_LINK === "" ||
    GLOBAL_CONSOLES_LINK === null
      ? null
      : GLOBAL_CONSOLES_LINK,
  agentMode: GLOBAL_AGENT_MODE === "true",
  ready: GLOBAL_READY === "true",
  pathPrefix: "",
  useLocalTime: false,
  enableQueryHistory: false,
  enableAutocomplete: true,
  enableSyntaxHighlighting: true,
  enableLinter: true,
  showAnnotations: false,
};

export const settingsSlice = createSlice({
  name: "settings",
  initialState,
  reducers: {
    updateSettings: (state, { payload }: PayloadAction<Partial<Settings>>) => {
      Object.assign(state, payload);
    },
  },
});

export const { updateSettings } = settingsSlice.actions;

export const useSettings = () => {
  return useAppSelector((state) => state.settings);
};

export default settingsSlice.reducer;
