import { createListenerMiddleware } from "@reduxjs/toolkit";
import { AppDispatch, RootState } from "./store";
import {
  localStorageKeyCollapsedPools,
  localStorageKeyTargetFilters,
  setCollapsedPools,
  updateTargetFilters,
} from "./targetsPageSlice";

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
  actionCreator: updateTargetFilters,
  effect: ({ payload }) => {
    persistToLocalStorage(localStorageKeyTargetFilters, payload);
  },
});
