import { createListenerMiddleware } from "@reduxjs/toolkit";
import { AppDispatch, RootState } from "./store";
import {
  localStorageKeyCollapsedPools as localStorageKeyTargetsPageCollapsedPools,
  setCollapsedPools as targetsPageSetCollapsedPools,
} from "./targetsPageSlice";
import {
  localStorageKeyCollapsedPools as localStorageKeyServiceDiscoveryPageCollapsedPools,
  setCollapsedPools as ServiceDiscoveryPageSetCollapsedPools,
} from "./serviceDiscoveryPageSlice";
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
  actionCreator: targetsPageSetCollapsedPools,
  effect: ({ payload }) => {
    persistToLocalStorage(localStorageKeyTargetsPageCollapsedPools, payload);
  },
});

startAppListening({
  actionCreator: ServiceDiscoveryPageSetCollapsedPools,
  effect: ({ payload }) => {
    persistToLocalStorage(
      localStorageKeyServiceDiscoveryPageCollapsedPools,
      payload
    );
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
