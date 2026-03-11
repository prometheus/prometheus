import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

export const localStorageKeyCollapsedPools = "serviceDiscovery.collapsedPools";
export const localStorageKeyTargetHealthFilter =
  "serviceDiscovery.healthFilter";

interface ServiceDiscoveryPage {
  collapsedPools: string[];
  showLimitAlert: boolean;
}

const initialState: ServiceDiscoveryPage = {
  collapsedPools: initializeFromLocalStorage<string[]>(
    localStorageKeyCollapsedPools,
    []
  ),
  showLimitAlert: false,
};

export const serviceDiscoveryPageSlice = createSlice({
  name: "serviceDiscoveryPage",
  initialState,
  reducers: {
    setCollapsedPools: (state, { payload }: PayloadAction<string[]>) => {
      state.collapsedPools = payload;
    },
    setShowLimitAlert: (state, { payload }: PayloadAction<boolean>) => {
      state.showLimitAlert = payload;
    },
  },
});

export const { setCollapsedPools, setShowLimitAlert } =
  serviceDiscoveryPageSlice.actions;

export default serviceDiscoveryPageSlice.reducer;
