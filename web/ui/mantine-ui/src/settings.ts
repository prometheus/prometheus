import { createContext } from "react";

export interface Settings {
  consolesLink: string | null;
  agentMode: boolean;
  ready: boolean;
}

export const SettingsContext = createContext<Settings>({
  consolesLink: null,
  agentMode: false,
  ready: false,
});
