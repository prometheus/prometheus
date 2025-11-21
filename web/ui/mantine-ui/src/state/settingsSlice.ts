import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { useAppSelector } from "./hooks";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

interface Settings {
  consolesLink: string | null;
  lookbackDelta: string;
  agentMode: boolean;
  ready: boolean;
  pathPrefix: string;
  useLocalTime: boolean;
  enableQueryHistory: boolean;
  enableAutocomplete: boolean;
  enableSyntaxHighlighting: boolean;
  enableLinter: boolean;
  showAnnotations: boolean;
  showQueryWarnings: boolean;
  showQueryInfoNotices: boolean;
  ruleGroupsPerPage: number;
  alertGroupsPerPage: number;
}

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_CONSOLES_LINK: string;
declare const GLOBAL_AGENT_MODE: string;
declare const GLOBAL_READY: string;
declare const GLOBAL_LOOKBACKDELTA: string;

export const localStorageKeyUseLocalTime = "settings.useLocalTime";
export const localStorageKeyEnableQueryHistory = "settings.enableQueryHistory";
export const localStorageKeyEnableAutocomplete = "settings.enableAutocomplete";
export const localStorageKeyEnableSyntaxHighlighting =
  "settings.enableSyntaxHighlighting";
export const localStorageKeyEnableLinter = "settings.enableLinter";
export const localStorageKeyShowAnnotations = "settings.showAnnotations";
export const localStorageKeyShowQueryWarnings = "settings.showQueryWarnings";
export const localStorageKeyShowQueryInfoNotices =
  "settings.showQueryInfoNotices";
export const localStorageKeyRuleGroupsPerPage = "settings.ruleGroupsPerPage";
export const localStorageKeyAlertGroupsPerPage = "settings.alertGroupsPerPage";

// This dynamically/generically determines the pathPrefix by stripping the first known
// endpoint suffix from the window location path. It works out of the box for both direct
// hosting and reverse proxy deployments with no additional configurations required.
const getPathPrefix = (path: string) => {
  if (path.endsWith("/")) {
    path = path.slice(0, -1);
  }

  const pagePaths = [
    "/query",
    "/alerts",
    "/targets",
    "/rules",
    "/service-discovery",
    "/status",
    "/tsdb-status",
    "/flags",
    "/config",
    "/alertmanager-discovery",
    "/agent",
  ];

  const pagePath = pagePaths.find((p) => path.endsWith(p));
  return path.slice(0, path.length - (pagePath || "").length);
};

export const initialState: Settings = {
  consolesLink:
    GLOBAL_CONSOLES_LINK === "CONSOLES_LINK_PLACEHOLDER" ||
    GLOBAL_CONSOLES_LINK === "" ||
    GLOBAL_CONSOLES_LINK === null
      ? null
      : GLOBAL_CONSOLES_LINK,
  agentMode: GLOBAL_AGENT_MODE === "true",
  ready: GLOBAL_READY === "true",
  lookbackDelta:
    GLOBAL_LOOKBACKDELTA === "LOOKBACKDELTA_PLACEHOLDER" ||
    GLOBAL_LOOKBACKDELTA === null
      ? ""
      : GLOBAL_LOOKBACKDELTA,
  pathPrefix: getPathPrefix(window.location.pathname),
  useLocalTime: initializeFromLocalStorage<boolean>(
    localStorageKeyUseLocalTime,
    false
  ),
  enableQueryHistory: initializeFromLocalStorage<boolean>(
    localStorageKeyEnableQueryHistory,
    false
  ),
  enableAutocomplete: initializeFromLocalStorage<boolean>(
    localStorageKeyEnableAutocomplete,
    true
  ),
  enableSyntaxHighlighting: initializeFromLocalStorage<boolean>(
    localStorageKeyEnableSyntaxHighlighting,
    true
  ),
  enableLinter: initializeFromLocalStorage<boolean>(
    localStorageKeyEnableLinter,
    true
  ),
  showAnnotations: initializeFromLocalStorage<boolean>(
    localStorageKeyShowAnnotations,
    true
  ),
  showQueryWarnings: initializeFromLocalStorage<boolean>(
    localStorageKeyShowQueryWarnings,
    true
  ),
  showQueryInfoNotices: initializeFromLocalStorage<boolean>(
    localStorageKeyShowQueryInfoNotices,
    true
  ),
  ruleGroupsPerPage: initializeFromLocalStorage<number>(
    localStorageKeyRuleGroupsPerPage,
    10
  ),
  alertGroupsPerPage: initializeFromLocalStorage<number>(
    localStorageKeyAlertGroupsPerPage,
    10
  ),
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
