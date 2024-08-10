import { createListenerMiddleware } from "@reduxjs/toolkit";
import { AppDispatch, RootState } from "./store";
import {
  localStorageKeyCollapsedPools,
  setCollapsedPools,
} from "./targetsPageSlice";
import { updateSettings } from "./settingsSlice";

const persistToLocalStorage = <T>(key: string, value: T) => {
  localStorage.setItem(key, JSON.stringify(value));
};

export const localStorageMiddleware = createListenerMiddleware();

const startAppListening = localStorageMiddleware.startListening.withTypes<
  RootState,
  AppDispatch
>();

startAppListening({
  actionCreator: setCollapsedPools,
  effect: ({ payload }) => {
    persistToLocalStorage(localStorageKeyCollapsedPools, payload);
  },
});

startAppListening({
  actionCreator: updateSettings,
  effect: ({ payload }) => {
    Object.entries(payload).forEach(([key, value]) => {
      switch (key) {
        case "useLocalTime":
        case "enableQueryHistory":
        case "enableAutocomplete":
        case "enableSyntaxHighlighting":
        case "enableLinter":
        case "showAnnotations":
          return persistToLocalStorage(`settings.${key}`, value);
      }
    });
  },
});
