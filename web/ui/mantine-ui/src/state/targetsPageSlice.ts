import { PayloadAction, createSlice } from "@reduxjs/toolkit";
import { initializeFromLocalStorage } from "./initializeFromLocalStorage";

export const localStorageKeyCollapsedPools = "targetsPage.collapsedPools";
export const localStorageKeyTargetHealthFilter = "targetsPage.healthFilter";

interface TargetsPage {
  selectedPool: string | null;
  healthFilter: string[];
  searchFilter: string;
  collapsedPools: string[];
}

const initialState: TargetsPage = {
  selectedPool: null,
  healthFilter: initializeFromLocalStorage<string[]>(
    localStorageKeyTargetHealthFilter,
    []
  ),
  searchFilter: "",
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
    setSearchFilter: (state, { payload }: PayloadAction<string>) => {
      state.searchFilter = payload;
    },
    setCollapsedPools: (state, { payload }: PayloadAction<string[]>) => {
      state.collapsedPools = payload;
    },
  },
});

export const {
  setSelectedPool,
  setHealthFilter,
  setSearchFilter,
  setCollapsedPools,
} = targetsPageSlice.actions;

export default targetsPageSlice.reducer;
