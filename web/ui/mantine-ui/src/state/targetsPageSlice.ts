import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

export const localStorageKeyCollapsedPools = "targetsPage.collapsedPools";
export const localStorageKeyTargetHealthFilter = "targetsPage.healthFilter";

interface TargetsPage {
  collapsedPools: string[];
  showLimitAlert: boolean;
}

const initialState: TargetsPage = {
  collapsedPools: initializeFromLocalStorage<string[]>(
    localStorageKeyCollapsedPools,
    []
  ),
  showLimitAlert: false,
};

export const targetsPageSlice = createSlice({
  name: "targetsPage",
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
  targetsPageSlice.actions;

export default targetsPageSlice.reducer;
