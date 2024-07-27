import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

export const localStorageKeyCollapsedPools = "targetsPage.collapsedPools";
export const localStorageKeyTargetHealthFilter = "targetsPage.healthFilter";

interface TargetsPage {
  selectedPool: string | null;
  healthFilter: string[];
  collapsedPools: string[];
}

const initialState: TargetsPage = {
  selectedPool: null,
  healthFilter: initializeFromLocalStorage<string[]>(
    localStorageKeyTargetHealthFilter,
    []
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
    setSelectedPool: (state, { payload }: PayloadAction<string | null>) => {
      state.selectedPool = payload;
    },
    setHealthFilter: (state, { payload }: PayloadAction<string[]>) => {
      state.healthFilter = payload;
    },
    setCollapsedPools: (state, { payload }: PayloadAction<string[]>) => {
      state.collapsedPools = payload;
    },
  },
});

export const { setSelectedPool, setHealthFilter, setCollapsedPools } =
  targetsPageSlice.actions;

export default targetsPageSlice.reducer;
