import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App.tsx";
import { Settings, SettingsContext } from "./settings.ts";
import store from "./state/store.ts";
import { Provider } from "react-redux";
import "./fonts/codicon.ttf";

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_CONSOLES_LINK: string;
declare const GLOBAL_AGENT_MODE: string;
declare const GLOBAL_READY: string;

const settings: Settings = {
  consolesLink:
    GLOBAL_CONSOLES_LINK === "CONSOLES_LINK_PLACEHOLDER" ||
    GLOBAL_CONSOLES_LINK === "" ||
    GLOBAL_CONSOLES_LINK === null
      ? null
      : GLOBAL_CONSOLES_LINK,
  agentMode: GLOBAL_AGENT_MODE === "true",
  ready: GLOBAL_READY === "true",
};

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <SettingsContext.Provider value={settings}>
      <Provider store={store}>
        <App />
      </Provider>
    </SettingsContext.Provider>
  </React.StrictMode>
);
