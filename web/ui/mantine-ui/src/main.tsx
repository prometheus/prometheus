import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App.tsx";
import store from "./state/store.ts";
import { Provider } from "react-redux";
import "./fonts/codicon.ttf";
import "./promql.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Provider store={store}>
      <App />
    </Provider>
  </React.StrictMode>
);
