import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

export const localStorageKeyCollapsedPools = "targetsPage.collapsedPools";
export const localStorageKeyTargetFilters = "targetsPage.filters";

interface TargetFilters {
  scrapePool: string | null;
  health: string[];
}

interface TargetsPage {
  filters: TargetFilters;
  collapsedPools: string[];
}

const initialState: TargetsPage = {
  filters: initializeFromLocalStorage<TargetFilters>(
    localStorageKeyTargetFilters,
    {
      scrapePool: null,
      health: [],
    }
  ),
  collapsedPools: initializeFromLocalStorage<string[]>(
    localStorageKeyCollapsedPools,
    []
  ),
};

export const targetsPageSlice = createSlice({
  name: "targetsPage",
  initialState,
  reducers: {
    updateTargetFilters: (
      state,
      { payload }: PayloadAction<Partial<TargetFilters>>
    ) => {
      Object.assign(state.filters, payload);
    },
    setCollapsedPools: (state, { payload }: PayloadAction<string[]>) => {
      state.collapsedPools = payload;
    },
  },
});

export const { updateTargetFilters, setCollapsedPools } =
  targetsPageSlice.actions;

export default targetsPageSlice.reducer;
