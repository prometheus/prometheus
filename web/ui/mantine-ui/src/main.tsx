import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App.tsx";
import store from "./state/store.ts";
import { Provider } from "react-redux";
import "./fonts/codicon.ttf";
import "./promql.css";
import { I18nProvider } from "./i18n.tsx";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <Provider store={store}>
        <App />
      </Provider>
    </I18nProvider>
  </React.StrictMode>
);
